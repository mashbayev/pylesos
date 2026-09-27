// Файл: internal/sources/repository/sqlite.go
package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jmoiron/sqlx"

	"pylesos/internal/sources/domain"
)

type SQLite struct{ db *sqlx.DB }

func NewSQLite(db *sqlx.DB) *SQLite { return &SQLite{db: db} }

func (r *SQLite) Ensure(ctx context.Context, s domain.Source) (int64, bool, error) {
	res, err := r.db.NamedExecContext(ctx, `INSERT INTO sources (kind, title, config, created_at)
		VALUES (:kind, :title, :config, :created_at) ON CONFLICT (kind, config) DO NOTHING`, s)
	if err != nil {
		return 0, false, fmt.Errorf("добавить источник: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, false, fmt.Errorf("добавить источник: %w", err)
	}
	if n == 1 {
		id, err := res.LastInsertId()
		if err != nil {
			return 0, false, fmt.Errorf("id источника: %w", err)
		}
		return id, true, nil
	}
	var id int64
	if err := r.db.GetContext(ctx, &id, `SELECT id FROM sources WHERE kind = ? AND config = ?`, s.Kind, s.Config); err != nil {
		return 0, false, fmt.Errorf("найти источник: %w", err)
	}
	return id, false, nil
}

func (r *SQLite) Get(ctx context.Context, id int64) (domain.Source, error) {
	var s domain.Source
	err := r.db.GetContext(ctx, &s, `SELECT id, kind, title, config, created_at FROM sources WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return s, domain.ErrNotFound
	}
	if err != nil {
		return s, fmt.Errorf("прочитать источник %d: %w", id, err)
	}
	return s, nil
}

func (r *SQLite) ListByKind(ctx context.Context, kind domain.Kind) ([]domain.Source, error) {
	list := []domain.Source{}
	if err := r.db.SelectContext(ctx, &list, `SELECT id, kind, title, config, created_at FROM sources
		WHERE kind = ? ORDER BY title, id`, kind); err != nil {
		return nil, fmt.Errorf("список источников: %w", err)
	}
	return list, nil
}
