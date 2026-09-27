// Файл: internal/jobs/repository/repository.go
// Package repository — хранение заданий.
package repository

import (
	"context"

	"pylesos/internal/jobs/domain"
)

type Repository interface {
	Create(ctx context.Context, j domain.Job) (int64, error)
	Get(ctx context.Context, id int64) (domain.Job, error)
	List(ctx context.Context) ([]domain.ListItem, error)
	ListByStatus(ctx context.Context, statuses ...domain.Status) ([]domain.Job, error)
	UpdateState(ctx context.Context, id int64, u domain.StateUpdate) error
	SaveCursor(ctx context.Context, id int64, cursor, progressText string, at int64) error
	SourceStats(ctx context.Context) (map[int64]domain.SourceStats, error)
}
