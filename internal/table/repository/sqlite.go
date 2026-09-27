// Файл: internal/table/repository/sqlite.go
package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jmoiron/sqlx"

	"pylesos/internal/table/domain"
	"pylesos/internal/tables"
)

type SQLite struct{ db *sqlx.DB }

func NewSQLite(db *sqlx.DB) *SQLite { return &SQLite{db: db} }

// Config — сохранённые колонки; ничего не сохранено — ok = false.
func (r *SQLite) Config(ctx context.Context) (domain.Config, bool, error) {
	var row struct {
		Columns        string `db:"columns"`
		IncludeQueries bool   `db:"include_queries"`
	}
	err := r.db.GetContext(ctx, &row, `SELECT columns, include_queries FROM table_config WHERE id = 1`)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Config{}, false, nil
	}
	if err != nil {
		return domain.Config{}, false, fmt.Errorf("колонки таблицы: %w", err)
	}
	cfg := domain.Config{IncludeQueries: row.IncludeQueries}
	if err := json.Unmarshal([]byte(row.Columns), &cfg.Columns); err != nil {
		return domain.Config{}, false, fmt.Errorf("колонки таблицы: %w", err)
	}
	return cfg, true, nil
}

func (r *SQLite) SaveConfig(ctx context.Context, cfg domain.Config) error {
	cols, err := json.Marshal(cfg.Columns)
	if err != nil {
		return fmt.Errorf("колонки таблицы: %w", err)
	}
	if _, err := r.db.ExecContext(ctx, `INSERT INTO table_config (id, columns, include_queries) VALUES (1, ?, ?)
		ON CONFLICT (id) DO UPDATE SET columns = excluded.columns, include_queries = excluded.include_queries`,
		string(cols), cfg.IncludeQueries); err != nil {
		return fmt.Errorf("сохранить колонки таблицы: %w", err)
	}
	return nil
}

// Row — готовая строка человека, если отпечаток совпал.
func (r *SQLite) Row(ctx context.Context, personID int64, hash string) (tables.Row, bool, error) {
	var data string
	err := r.db.GetContext(ctx, &data, `SELECT row FROM table_rows WHERE person_id = ? AND hash = ?`, personID, hash)
	if errors.Is(err, sql.ErrNoRows) {
		return tables.Row{}, false, nil
	}
	if err != nil {
		return tables.Row{}, false, fmt.Errorf("строка человека %d: %w", personID, err)
	}
	var row tables.Row
	if err := json.Unmarshal([]byte(data), &row); err != nil {
		return tables.Row{}, false, fmt.Errorf("строка человека %d: %w", personID, err)
	}
	return row, true, nil
}

func (r *SQLite) SaveRow(ctx context.Context, personID int64, hash string, row tables.Row) error {
	data, err := json.Marshal(row)
	if err != nil {
		return fmt.Errorf("строка человека %d: %w", personID, err)
	}
	if _, err := r.db.ExecContext(ctx, `INSERT INTO table_rows (person_id, hash, row) VALUES (?, ?, ?)
		ON CONFLICT (person_id) DO UPDATE SET hash = excluded.hash, row = excluded.row`, personID, hash, string(data)); err != nil {
		return fmt.Errorf("сохранить строку человека %d: %w", personID, err)
	}
	return nil
}

// People — люди, упомянутые в группе; без записей (только «есть инфо?») — только если includeQueries (П20).
func (r *SQLite) People(ctx context.Context, sourceID int64, includeQueries bool) ([]domain.Person, error) {
	list := []domain.Person{}
	if err := r.db.SelectContext(ctx, &list, `SELECT p.id, p.full_name, COALESCE(p.iin, '') AS iin, p.doc_number, p.birth_date
		FROM persons p WHERE EXISTS (SELECT 1 FROM mentions m WHERE m.person_id = p.id AND m.source_id = ?)
		AND (? OR EXISTS (SELECT 1 FROM mentions m WHERE m.person_id = p.id AND m.role <> 'query'))
		ORDER BY p.full_name, p.id`, sourceID, includeQueries); err != nil {
		return nil, fmt.Errorf("люди группы %d: %w", sourceID, err)
	}
	return list, nil
}

// DocTexts — распознанные тексты документов из сообщений о человеке.
func (r *SQLite) DocTexts(ctx context.Context, personID int64) ([]string, error) {
	list := []string{}
	if err := r.db.SelectContext(ctx, &list, `SELECT md.recognized_text FROM mentions m
		JOIN tg_media md ON md.source_id = m.source_id AND md.msg_id = m.msg_id
		WHERE m.person_id = ? AND md.recognized_text <> ''
		GROUP BY md.source_id, md.msg_id, md.idx ORDER BY md.source_id, md.msg_id, md.idx`, personID); err != nil {
		return nil, fmt.Errorf("документы человека %d: %w", personID, err)
	}
	return list, nil
}
