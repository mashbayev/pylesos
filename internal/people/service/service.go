// Файл: internal/people/service/service.go
// Package service — действия пользователя с людьми: склейка, отказ от склейки, ручная привязка (П17).
package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"pylesos/internal/people/domain"
	"pylesos/internal/people/names"
)

type Repo interface {
	Person(ctx context.Context, id int64) (domain.Person, error)
	Merge(ctx context.Context, keep, drop int64) error
	Reject(ctx context.Context, a, b int64) error
	LinkMention(ctx context.Context, mentionID, personID int64) error
	CreatePerson(ctx context.Context, p domain.Person) (int64, error)
}

var ErrNoName = errors.New("у упоминания нет имени")

type Service struct {
	repo Repo
	now  func() time.Time
}

func New(repo Repo, now func() time.Time) *Service { return &Service{repo: repo, now: now} }

// Merge — «это один человек»: остаётся тот, у кого известен ИИН (иначе тот, кто раньше создан).
func (s *Service) Merge(ctx context.Context, a, b int64) error {
	pa, err := s.repo.Person(ctx, a)
	if err != nil {
		return err
	}
	pb, err := s.repo.Person(ctx, b)
	if err != nil {
		return err
	}
	keep, drop := min(a, b), max(a, b)
	if pa.IIN.Valid != pb.IIN.Valid {
		keep, drop = a, b
		if pb.IIN.Valid {
			keep, drop = b, a
		}
	}
	return s.repo.Merge(ctx, keep, drop)
}

func (s *Service) Reject(ctx context.Context, a, b int64) error { return s.repo.Reject(ctx, a, b) }

// Link — привязать упоминание к человеку; personID == 0 — к новому человеку с именем из упоминания.
func (s *Service) Link(ctx context.Context, mentionID, personID int64, name string) error {
	if personID == 0 {
		key := names.Key(name)
		if key == "" {
			return ErrNoName
		}
		id, err := s.repo.CreatePerson(ctx, domain.Person{FullName: strings.TrimSpace(name), NameKey: key, CreatedAt: s.now().Unix()})
		if err != nil {
			return err
		}
		personID = id
	}
	return s.repo.LinkMention(ctx, mentionID, personID)
}
