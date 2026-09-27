// Файл: internal/docforms/service/service.go
// Package service — формы документов: образец, разметка рамки и зон (П21).
package service

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"pylesos/internal/docforms/domain"
	"pylesos/internal/docforms/repository"
	"pylesos/internal/normalize"
	"pylesos/internal/recognize"
	"pylesos/internal/tables"
)

// PageMaker — файл → страницы (normalize.Normalizer).
type PageMaker interface {
	Bytes(name string, data []byte) ([]normalize.Page, error)
}

type Service struct {
	repo  repository.Repository
	pages PageMaker
	now   func() time.Time
}

func New(repo repository.Repository, pages PageMaker, now func() time.Time) *Service {
	return &Service{repo: repo, pages: pages, now: now}
}

// Create — новая форма из образца: первая страница файла с изображением.
func (s *Service) Create(ctx context.Context, name, fileName string, data []byte) (int64, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return 0, domain.ErrNoName
	}
	sample, err := s.firstImage(fileName, data)
	if err != nil {
		return 0, err
	}
	sig, err := recognize.Signature(sample)
	if err != nil {
		return 0, fmt.Errorf("%w: %v", domain.ErrNoSample, err)
	}
	return s.repo.Create(ctx, domain.Form{Name: name, Sample: sample, Signature: sig,
		Zones: map[string]tables.Zone{}, UpdatedAt: s.now().Unix()})
}

func (s *Service) firstImage(fileName string, data []byte) ([]byte, error) {
	pages, err := s.pages.Bytes(fileName, data)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", domain.ErrNoSample, err) // причина (пароль, формат) нужна экрану
	}
	for _, p := range pages {
		if p.Image != nil {
			return p.Image, nil
		}
	}
	return nil, domain.ErrNoSample
}

// SetZone отмечает зону поля или рамку карточки (FrameKey — рамка, по ней строится Locator).
func (s *Service) SetZone(ctx context.Context, id int64, key string, z tables.Zone) error {
	if !z.Valid() {
		return domain.ErrBadZone
	}
	f, err := s.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	switch {
	case key == domain.FrameKey:
		loc, err := recognize.Locator(f.Sample, z)
		if err != nil {
			return fmt.Errorf("%w: %v", domain.ErrBadZone, err)
		}
		f.Frame, f.Locator = &z, loc
	case slices.Contains(domain.Fields(), key):
		f.Zones[key] = z
	default:
		return domain.ErrBadField
	}
	return s.save(ctx, f)
}

// ClearZone снимает отметку поля или рамки.
func (s *Service) ClearZone(ctx context.Context, id int64, key string) error {
	f, err := s.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	switch {
	case key == domain.FrameKey:
		f.Frame, f.Locator = nil, ""
	case slices.Contains(domain.Fields(), key):
		delete(f.Zones, key)
	default:
		return domain.ErrBadField
	}
	return s.save(ctx, f)
}

func (s *Service) Rename(ctx context.Context, id int64, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return domain.ErrNoName
	}
	f, err := s.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	f.Name = name
	return s.save(ctx, f)
}

func (s *Service) save(ctx context.Context, f domain.Form) error {
	f.UpdatedAt = s.now().Unix()
	return s.repo.Update(ctx, f)
}

func (s *Service) Get(ctx context.Context, id int64) (domain.Form, error) { return s.repo.Get(ctx, id) }
func (s *Service) List(ctx context.Context) ([]domain.Form, error)        { return s.repo.List(ctx) }
func (s *Service) Delete(ctx context.Context, id int64) error             { return s.repo.Delete(ctx, id) }

// Sample — JPEG образца формы (для разметки на экране).
func (s *Service) Sample(ctx context.Context, id int64) ([]byte, error) {
	f, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return f.Sample, nil
}

// IsUserError — ошибка, которую пользователь исправит сам (показывается текстом, а не «внутренней ошибкой»).
func IsUserError(err error) bool {
	return errors.Is(err, domain.ErrNoName) || errors.Is(err, domain.ErrNoSample) ||
		errors.Is(err, domain.ErrBadZone) || errors.Is(err, domain.ErrBadField) || errors.Is(err, domain.ErrNotFound)
}
