// Файл: internal/settings/service/service.go
// Package service — чтение и проверка настроек.
package service

import (
	"context"
	"fmt"
	"strconv"

	"pylesos/internal/settings/domain"
	"pylesos/internal/settings/repository"
)

type Service struct {
	repo     repository.Repository
	defaults domain.Settings
}

func NewService(repo repository.Repository, defaults domain.Settings) *Service {
	return &Service{repo: repo, defaults: defaults}
}

// Get — сохранённые настройки поверх значений по умолчанию; испорченное значение заменяется умолчанием.
func (s *Service) Get(ctx context.Context) (domain.Settings, error) {
	stored, err := s.repo.All(ctx)
	if err != nil {
		return domain.Settings{}, fmt.Errorf("настройки: %w", err)
	}
	out := s.defaults
	if v := stored[domain.KeyExportsDir]; v != "" {
		out.ExportsDir = v
	}
	if n, convErr := strconv.Atoi(stored[domain.KeyPhonePauseSec]); convErr == nil && inPauseRange(n) {
		out.PhonePauseSec = n
	}
	return out, nil
}

// Save проверяет ввод пользователя и сохраняет его. Ошибки проверки возвращаются по ключам полей
// готовым русским текстом; при них ничего не сохраняется, а первый результат — текущие настройки.
func (s *Service) Save(ctx context.Context, input map[string]string) (domain.Settings, map[string]string, error) {
	values, problems := validate(input)
	if len(problems) > 0 {
		current, err := s.Get(ctx)
		return current, problems, err
	}
	if err := s.repo.Put(ctx, values); err != nil {
		return domain.Settings{}, nil, fmt.Errorf("сохранить настройки: %w", err)
	}
	saved, err := s.Get(ctx)
	return saved, nil, err
}
