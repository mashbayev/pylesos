// Файл: internal/settings/repository/sqlite.go
package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jmoiron/sqlx"
)

// SQLite — настройки в таблице settings.
type SQLite struct{ db *sqlx.DB }

func NewSQLite(db *sqlx.DB) *SQLite { return &SQLite{db: db} }

type row struct {
	Key   string `db:"key"`
	Value string `db:"value"`
}

func (r *SQLite) All(ctx context.Context) (map[string]string, error) {
	var rows []row
	if err := r.db.SelectContext(ctx, &rows, `SELECT key, value FROM settings`); err != nil {
		return nil, fmt.Errorf("прочитать настройки: %w", err)
	}
	out := make(map[string]string, len(rows))
	for _, x := range rows {
		out[x.Key] = x.Value
	}
	return out, nil
}

func (r *SQLite) Put(ctx context.Context, values map[string]string) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("начать запись настроек: %w", err)
	}
	for k, v := range values {
		if _, err := tx.ExecContext(ctx, `INSERT INTO settings (key, value) VALUES (?, ?)
			ON CONFLICT (key) DO UPDATE SET value = excluded.value`, k, v); err != nil {
			return errors.Join(fmt.Errorf("записать настройку %s: %w", k, err), tx.Rollback())
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("сохранить настройки: %w", err)
	}
	return nil
}
