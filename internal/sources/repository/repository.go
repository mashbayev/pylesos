// Файл: internal/sources/repository/repository.go
// Package repository — хранение источников.
package repository

import (
	"context"

	"pylesos/internal/sources/domain"
)

type Repository interface {
	// Ensure создаёт источник или возвращает существующий с тем же kind и config; bool — создан.
	Ensure(ctx context.Context, s domain.Source) (int64, bool, error)
	Get(ctx context.Context, id int64) (domain.Source, error)
	ListByKind(ctx context.Context, kind domain.Kind) ([]domain.Source, error)
}
