// Файл: internal/store/storetest/storetest.go
// Package storetest — временная база для тестов других пакетов.
package storetest

import (
	"context"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/jmoiron/sqlx"

	"pylesos/internal/store"
)

// Open создаёт пустую базу со всеми миграциями; закрывается после теста.
func Open(t *testing.T) *sqlx.DB {
	t.Helper()
	db, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("открыть тестовую базу: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("закрыть тестовую базу: %v", err)
		}
	})
	return db
}

// AddSource добавляет источник напрямую. config уникален по названию (индекс sources_kind_config).
func AddSource(t *testing.T, db *sqlx.DB, title string) int64 {
	t.Helper()
	config := `{"test":` + strconv.Quote(title) + `}`
	res, err := db.Exec(`INSERT INTO sources (kind, title, config, created_at) VALUES ('telegram_group', ?, ?, 0)`, title, config)
	if err != nil {
		t.Fatalf("добавить источник: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("id источника: %v", err)
	}
	return id
}
