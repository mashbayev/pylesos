// Файл: internal/docforms/repository/sqlite.go
// Package repository — хранение форм документов.
package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jmoiron/sqlx"

	"pylesos/internal/docforms/domain"
	"pylesos/internal/tables"
)

type Repository interface {
	Create(ctx context.Context, f domain.Form) (int64, error)
	Get(ctx context.Context, id int64) (domain.Form, error)
	List(ctx context.Context) ([]domain.Form, error)
	Update(ctx context.Context, f domain.Form) error
	Delete(ctx context.Context, id int64) error
}

type SQLite struct{ db *sqlx.DB }

func NewSQLite(db *sqlx.DB) *SQLite { return &SQLite{db: db} }

// row — форма в таблице doc_forms: рамка и зоны — JSON.
type row struct {
	ID        int64  `db:"id"`
	Name      string `db:"name"`
	Sample    []byte `db:"sample"`
	Signature string `db:"signature"`
	Frame     string `db:"frame"`
	Locator   string `db:"locator"`
	Zones     string `db:"zones"`
	UpdatedAt int64  `db:"updated_at"`
}

const columns = `id, name, sample, signature, frame, locator, zones, updated_at`

func toRow(f domain.Form) (row, error) {
	zones, err := json.Marshal(f.Zones)
	if err != nil {
		return row{}, fmt.Errorf("зоны формы: %w", err)
	}
	r := row{ID: f.ID, Name: f.Name, Sample: f.Sample, Signature: f.Signature, Locator: f.Locator, Zones: string(zones), UpdatedAt: f.UpdatedAt}
	if f.Frame != nil {
		frame, err := json.Marshal(f.Frame)
		if err != nil {
			return row{}, fmt.Errorf("рамка формы: %w", err)
		}
		r.Frame = string(frame)
	}
	return r, nil
}

func fromRow(r row) (domain.Form, error) {
	f := domain.Form{ID: r.ID, Name: r.Name, Sample: r.Sample, Signature: r.Signature, Locator: r.Locator,
		Zones: map[string]tables.Zone{}, UpdatedAt: r.UpdatedAt}
	if err := json.Unmarshal([]byte(r.Zones), &f.Zones); err != nil {
		return domain.Form{}, fmt.Errorf("зоны формы %d: %w", r.ID, err)
	}
	if r.Frame != "" {
		var z tables.Zone
		if err := json.Unmarshal([]byte(r.Frame), &z); err != nil {
			return domain.Form{}, fmt.Errorf("рамка формы %d: %w", r.ID, err)
		}
		f.Frame = &z
	}
	return f, nil
}

func (r *SQLite) Create(ctx context.Context, f domain.Form) (int64, error) {
	rw, err := toRow(f)
	if err != nil {
		return 0, err
	}
	res, err := r.db.NamedExecContext(ctx, `INSERT INTO doc_forms (name, sample, signature, frame, locator, zones, updated_at)
		VALUES (:name, :sample, :signature, :frame, :locator, :zones, :updated_at)`, rw)
	if err != nil {
		return 0, fmt.Errorf("создать форму: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("id формы: %w", err)
	}
	return id, nil
}

func (r *SQLite) Get(ctx context.Context, id int64) (domain.Form, error) {
	var rw row
	err := r.db.GetContext(ctx, &rw, `SELECT `+columns+` FROM doc_forms WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Form{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Form{}, fmt.Errorf("прочитать форму %d: %w", id, err)
	}
	return fromRow(rw)
}

func (r *SQLite) List(ctx context.Context) ([]domain.Form, error) {
	var rows []row
	if err := r.db.SelectContext(ctx, &rows, `SELECT `+columns+` FROM doc_forms ORDER BY name, id`); err != nil {
		return nil, fmt.Errorf("список форм: %w", err)
	}
	out := make([]domain.Form, 0, len(rows))
	for _, rw := range rows {
		f, err := fromRow(rw)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, nil
}

func (r *SQLite) Update(ctx context.Context, f domain.Form) error {
	rw, err := toRow(f)
	if err != nil {
		return err
	}
	res, err := r.db.NamedExecContext(ctx, `UPDATE doc_forms SET name = :name, frame = :frame, locator = :locator,
		zones = :zones, updated_at = :updated_at WHERE id = :id`, rw)
	if err != nil {
		return fmt.Errorf("обновить форму %d: %w", f.ID, err)
	}
	return affected(res, f.ID)
}

func (r *SQLite) Delete(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM doc_forms WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("удалить форму %d: %w", id, err)
	}
	return affected(res, id)
}

func affected(res sql.Result, id int64) error {
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("форма %d: %w", id, err)
	}
	if n == 0 {
		return domain.ErrNotFound
	}
	return nil
}
