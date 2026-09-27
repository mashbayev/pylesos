// Файл: internal/people/repository/persons.go
// Package repository — люди, упоминания и кандидаты на склейку.
package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jmoiron/sqlx"

	"pylesos/internal/people/domain"
)

type SQLite struct{ db *sqlx.DB }

func NewSQLite(db *sqlx.DB) *SQLite { return &SQLite{db: db} }

const personCols = `id, full_name, name_key, iin, doc_number, birth_date, created_at`

func (r *SQLite) CreatePerson(ctx context.Context, p domain.Person) (int64, error) {
	res, err := r.db.NamedExecContext(ctx, `INSERT INTO persons (full_name, name_key, iin, doc_number, birth_date, created_at)
		VALUES (:full_name, :name_key, :iin, :doc_number, :birth_date, :created_at)`, p)
	if err != nil {
		return 0, fmt.Errorf("создать человека: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("id человека: %w", err)
	}
	return id, nil
}

func (r *SQLite) Person(ctx context.Context, id int64) (domain.Person, error) {
	var p domain.Person
	err := r.db.GetContext(ctx, &p, `SELECT `+personCols+` FROM persons WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return p, domain.ErrNotFound
	}
	if err != nil {
		return p, fmt.Errorf("человек %d: %w", id, err)
	}
	return p, nil
}

// PersonByIIN — человек с этим ИИН; ok=false — нет.
func (r *SQLite) PersonByIIN(ctx context.Context, iin string) (domain.Person, bool, error) {
	p, err := r.one(ctx, `SELECT `+personCols+` FROM persons WHERE iin = ?`, iin)
	if errors.Is(err, domain.ErrNotFound) {
		return p, false, nil
	}
	return p, err == nil, err
}

func (r *SQLite) one(ctx context.Context, query string, args ...any) (domain.Person, error) {
	var p domain.Person
	err := r.db.GetContext(ctx, &p, query, args...)
	if errors.Is(err, sql.ErrNoRows) {
		return p, domain.ErrNotFound
	}
	if err != nil {
		return p, fmt.Errorf("поиск человека: %w", err)
	}
	return p, nil
}

func (r *SQLite) PersonsByKey(ctx context.Context, key string) ([]domain.Person, error) {
	list := []domain.Person{}
	if err := r.db.SelectContext(ctx, &list, `SELECT `+personCols+` FROM persons WHERE name_key = ? ORDER BY id`, key); err != nil {
		return nil, fmt.Errorf("люди по ФИО: %w", err)
	}
	return list, nil
}

// Keys — id, ключ ФИО и ИИН всех людей (для поиска похожих).
func (r *SQLite) Keys(ctx context.Context) ([]domain.Person, error) {
	list := []domain.Person{}
	if err := r.db.SelectContext(ctx, &list, `SELECT id, name_key, iin FROM persons`); err != nil {
		return nil, fmt.Errorf("ключи ФИО: %w", err)
	}
	return list, nil
}

// Fill дополняет пустые поля человека; непустое не затирается (02-database, COALESCE).
func (r *SQLite) Fill(ctx context.Context, id int64, p domain.Person) error {
	if _, err := r.db.ExecContext(ctx, `UPDATE persons SET
		iin        = COALESCE(iin, ?),
		doc_number = CASE WHEN doc_number = '' THEN ? ELSE doc_number END,
		birth_date = CASE WHEN birth_date = '' THEN ? ELSE birth_date END
		WHERE id = ?`, p.IIN, p.DocNumber, p.BirthDate, id); err != nil {
		return fmt.Errorf("дополнить человека %d: %w", id, err)
	}
	return nil
}
