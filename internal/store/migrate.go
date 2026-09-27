// Файл: internal/store/migrate.go
package store

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"sort"

	"github.com/jmoiron/sqlx"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// migrate применяет по порядку миграции, которых ещё нет в schema_migrations.
func migrate(ctx context.Context, db *sqlx.DB) error {
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version TEXT PRIMARY KEY)`); err != nil {
		return fmt.Errorf("таблица миграций: %w", err)
	}
	names, err := fs.Glob(migrationsFS, "migrations/*.sql")
	if err != nil {
		return fmt.Errorf("список миграций: %w", err)
	}
	sort.Strings(names)
	for _, name := range names {
		if err := applyOne(ctx, db, name); err != nil {
			return err
		}
	}
	return nil
}

func applyOne(ctx context.Context, db *sqlx.DB, name string) error {
	version := path.Base(name)
	var n int
	if err := db.GetContext(ctx, &n, `SELECT COUNT(*) FROM schema_migrations WHERE version = ?`, version); err != nil {
		return fmt.Errorf("проверить миграцию %s: %w", version, err)
	}
	if n > 0 {
		return nil
	}
	body, err := migrationsFS.ReadFile(name)
	if err != nil {
		return fmt.Errorf("прочитать миграцию %s: %w", version, err)
	}
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("начать миграцию %s: %w", version, err)
	}
	if _, err := tx.ExecContext(ctx, string(body)); err != nil {
		return errors.Join(fmt.Errorf("применить миграцию %s: %w", version, err), tx.Rollback())
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations (version) VALUES (?)`, version); err != nil {
		return errors.Join(fmt.Errorf("отметить миграцию %s: %w", version, err), tx.Rollback())
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("завершить миграцию %s: %w", version, err)
	}
	return nil
}
