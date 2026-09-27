// Файл: internal/store/store_test.go
package store

import (
	"context"
	"io/fs"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"
)

func closeDB(t *testing.T, db *sqlx.DB) {
	t.Helper()
	if err := db.Close(); err != nil {
		t.Errorf("закрыть базу: %v", err)
	}
}

func TestOpenCyrillicPathAppliesMigrationsOnce(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "Пылесос", "база.db")
	db, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("первое открытие: %v", err)
	}
	closeDB(t, db)
	db, err = Open(ctx, path)
	if err != nil {
		t.Fatalf("повторное открытие: %v", err)
	}
	defer closeDB(t, db)
	var tables []string
	if err := db.SelectContext(ctx, &tables, `SELECT name FROM sqlite_master
		WHERE type = 'table' AND name IN ('settings', 'sources', 'jobs') ORDER BY name`); err != nil {
		t.Fatal(err)
	}
	if len(tables) != 3 {
		t.Fatalf("таблицы: %v", tables)
	}
	var applied int
	if err := db.GetContext(ctx, &applied, `SELECT COUNT(*) FROM schema_migrations`); err != nil {
		t.Fatal(err)
	}
	names, err := fs.Glob(migrationsFS, "migrations/*.sql")
	if err != nil {
		t.Fatal(err)
	}
	if applied != len(names) {
		t.Fatalf("применено миграций: %d, файлов: %d", applied, len(names))
	}
}

func TestForeignKeysOn(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer closeDB(t, db)
	_, err = db.ExecContext(ctx, `INSERT INTO jobs (source_id, kind, status, created_at, updated_at)
		VALUES (999, 'x', 'queued', 0, 0)`)
	if err == nil {
		t.Fatal("задание без источника вставилось — внешние ключи выключены")
	}
}

func TestOneActiveJobPerSourceIndex(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer closeDB(t, db)
	if _, err := db.ExecContext(ctx, `INSERT INTO sources (id, kind, title, created_at) VALUES (1, 'site', 'Проверка', 0)`); err != nil {
		t.Fatal(err)
	}
	insert := `INSERT INTO jobs (source_id, kind, status, created_at, updated_at) VALUES (1, 'x', ?, 0, 0)`
	if _, err := db.ExecContext(ctx, insert, "done"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, insert, "running"); err != nil {
		t.Fatalf("активное после завершённого: %v", err)
	}
	if _, err := db.ExecContext(ctx, insert, "paused"); err == nil {
		t.Fatal("второе активное задание на источник вставилось")
	}
}

// Время ставит Go, не SQL (02-database.md): источник без created_at не вставляется.
func TestSourcesCreatedAtSetByGo(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer closeDB(t, db)
	if _, err := db.ExecContext(ctx, `INSERT INTO sources (kind, title) VALUES ('site', 'Проверка')`); err == nil {
		t.Fatal("источник без created_at вставился — время поставил SQL")
	}
}
