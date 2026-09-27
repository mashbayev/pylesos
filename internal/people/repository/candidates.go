// Файл: internal/people/repository/candidates.go
package repository

import (
	"context"
	"errors"
	"fmt"

	"pylesos/internal/people/domain"
)

func ordered(a, b int64) (int64, int64) { return min(a, b), max(a, b) }

// AddCandidate добавляет пару «возможно, один человек»; уже известная (в т.ч. отклонённая) — не меняется.
func (r *SQLite) AddCandidate(ctx context.Context, a, b int64) error {
	a, b = ordered(a, b)
	if _, err := r.db.ExecContext(ctx, `INSERT INTO merge_candidates (a, b) VALUES (?, ?) ON CONFLICT (a, b) DO NOTHING`, a, b); err != nil {
		return fmt.Errorf("кандидат на склейку: %w", err)
	}
	return nil
}

func (r *SQLite) Candidates(ctx context.Context) ([]domain.Candidate, error) {
	list := []domain.Candidate{}
	if err := r.db.SelectContext(ctx, &list, `SELECT a, b FROM merge_candidates WHERE status = 'pending' ORDER BY a, b`); err != nil {
		return nil, fmt.Errorf("кандидаты на склейку: %w", err)
	}
	return list, nil
}

func (r *SQLite) Reject(ctx context.Context, a, b int64) error {
	a, b = ordered(a, b)
	if _, err := r.db.ExecContext(ctx, `UPDATE merge_candidates SET status = 'rejected' WHERE a = ? AND b = ?`, a, b); err != nil {
		return fmt.Errorf("отклонить пару: %w", err)
	}
	return nil
}

// Merge склеивает drop в keep: упоминания переходят к keep, пустые поля keep дополняются, drop удаляется.
func (r *SQLite) Merge(ctx context.Context, keep, drop int64) error {
	d, err := r.Person(ctx, drop)
	if err != nil {
		return err
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("начать склейку: %w", err)
	}
	steps := []struct {
		query string
		args  []any
	}{
		{`UPDATE mentions SET person_id = ? WHERE person_id = ?`, []any{keep, drop}},
		{`DELETE FROM merge_candidates WHERE a = ? OR b = ?`, []any{drop, drop}},
		{`DELETE FROM persons WHERE id = ?`, []any{drop}}, // до переноса ИИН: он уникален
		{`UPDATE persons SET iin = COALESCE(iin, ?),
			doc_number = CASE WHEN doc_number = '' THEN ? ELSE doc_number END,
			birth_date = CASE WHEN birth_date = '' THEN ? ELSE birth_date END WHERE id = ?`,
			[]any{d.IIN, d.DocNumber, d.BirthDate, keep}},
	}
	for _, s := range steps {
		if _, err := tx.ExecContext(ctx, s.query, s.args...); err != nil {
			return errors.Join(fmt.Errorf("склейка %d ← %d: %w", keep, drop, err), tx.Rollback())
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("завершить склейку: %w", err)
	}
	return nil
}
