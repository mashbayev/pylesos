// Файл: internal/store/db.go
// Package store открывает базу SQLite (без CGO, П14) и применяет миграции.
package store

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite" // регистрирует драйвер "sqlite"
)

const pragmas = "?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"

// Open открывает (или создаёт) базу по пути path и применяет миграции.
func Open(ctx context.Context, path string) (*sqlx.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("папка базы: %w", err)
	}
	db, err := sqlx.Open("sqlite", path+pragmas)
	if err != nil {
		return nil, fmt.Errorf("открыть базу: %w", err)
	}
	db.SetMaxOpenConns(1) // один писатель: SQLite не любит параллельную запись
	if err := migrate(ctx, db); err != nil {
		return nil, errors.Join(err, db.Close())
	}
	return db, nil
}
