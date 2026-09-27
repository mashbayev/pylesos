// Файл: internal/jobs/repository/sqlite.go
package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jmoiron/sqlx"
	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"

	"pylesos/internal/jobs/domain"
)

const columns = `id, source_id, kind, params, status, cursor, progress_text, reason, created_at, updated_at, finished_at`

type SQLite struct{ db *sqlx.DB }

func NewSQLite(db *sqlx.DB) *SQLite { return &SQLite{db: db} }

func (r *SQLite) Create(ctx context.Context, j domain.Job) (int64, error) {
	res, err := r.db.NamedExecContext(ctx, `INSERT INTO jobs (source_id, kind, params, status, cursor, created_at, updated_at)
		VALUES (:source_id, :kind, :params, :status, :cursor, :created_at, :updated_at)`, j)
	if err != nil {
		return 0, busyOr(fmt.Errorf("создать задание: %w", err))
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("id задания: %w", err)
	}
	return id, nil
}

func (r *SQLite) Get(ctx context.Context, id int64) (domain.Job, error) {
	var j domain.Job
	err := r.db.GetContext(ctx, &j, `SELECT `+columns+` FROM jobs WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return j, domain.ErrNotFound
	}
	if err != nil {
		return j, fmt.Errorf("прочитать задание %d: %w", id, err)
	}
	return j, nil
}

func (r *SQLite) List(ctx context.Context) ([]domain.ListItem, error) {
	items := []domain.ListItem{}
	err := r.db.SelectContext(ctx, &items, `SELECT j.id, j.source_id, j.kind, j.params, j.status, j.cursor,
		j.progress_text, j.reason, j.created_at, j.updated_at, j.finished_at, s.title AS source_title
		FROM jobs j JOIN sources s ON s.id = j.source_id ORDER BY j.id DESC`)
	if err != nil {
		return nil, fmt.Errorf("список заданий: %w", err)
	}
	return items, nil
}

func (r *SQLite) ListByStatus(ctx context.Context, statuses ...domain.Status) ([]domain.Job, error) {
	query, args, err := sqlx.In(`SELECT `+columns+` FROM jobs WHERE status IN (?) ORDER BY id`, statuses)
	if err != nil {
		return nil, fmt.Errorf("запрос заданий по статусу: %w", err)
	}
	jobs := []domain.Job{}
	if err := r.db.SelectContext(ctx, &jobs, r.db.Rebind(query), args...); err != nil {
		return nil, fmt.Errorf("задания по статусу: %w", err)
	}
	return jobs, nil
}

func (r *SQLite) UpdateState(ctx context.Context, id int64, u domain.StateUpdate) error {
	var finished sql.NullInt64
	if u.Finished {
		finished = sql.NullInt64{Int64: u.At, Valid: true}
	}
	query := `UPDATE jobs SET status = ?, reason = ?, updated_at = ?, finished_at = ? WHERE id = ?`
	args := []any{u.Status, u.Reason, u.At, finished, id}
	if len(u.From) > 0 {
		in, inArgs, err := sqlx.In(query+` AND status IN (?)`, append(args, u.From)...)
		if err != nil {
			return fmt.Errorf("условие перехода задания %d: %w", id, err)
		}
		query, args = in, inArgs
	}
	res, err := r.db.ExecContext(ctx, r.db.Rebind(query), args...)
	if err != nil {
		return busyOr(fmt.Errorf("обновить задание %d: %w", id, err))
	}
	if err := affected(res, id); !errors.Is(err, domain.ErrNotFound) || len(u.From) == 0 {
		return err
	}
	return r.missingOrChanged(ctx, id)
}

// missingOrChanged: условный переход не сработал — задания нет или его статус уже другой.
func (r *SQLite) missingOrChanged(ctx context.Context, id int64) error {
	if _, err := r.Get(ctx, id); err != nil {
		return err
	}
	return domain.ErrStateChanged
}

func (r *SQLite) SaveCursor(ctx context.Context, id int64, cursor, progressText string, at int64) error {
	res, err := r.db.ExecContext(ctx, `UPDATE jobs SET cursor = ?, progress_text = ?, updated_at = ? WHERE id = ?`,
		cursor, progressText, at, id)
	if err != nil {
		return fmt.Errorf("сохранить курсор задания %d: %w", id, err)
	}
	return affected(res, id)
}

func affected(res sql.Result, id int64) error {
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("задание %d: %w", id, err)
	}
	if n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// busyOr превращает нарушение индекса jobs_one_active_per_source в domain.ErrSourceBusy.
func busyOr(err error) error {
	var se *sqlite.Error
	if errors.As(err, &se) && se.Code() == sqlite3.SQLITE_CONSTRAINT_UNIQUE {
		return fmt.Errorf("%w: %w", domain.ErrSourceBusy, err)
	}
	return err
}

func (r *SQLite) SourceStats(ctx context.Context) (map[int64]domain.SourceStats, error) {
	var rows []domain.SourceStats
	if err := r.db.SelectContext(ctx, &rows, `SELECT source_id,
		MAX(status IN ('queued', 'running', 'paused', 'waiting_user')) AS active,
		COALESCE(MAX(CASE WHEN status = 'done' THEN finished_at END), 0) AS last_done
		FROM jobs GROUP BY source_id`); err != nil {
		return nil, fmt.Errorf("сводка заданий по источникам: %w", err)
	}
	out := make(map[int64]domain.SourceStats, len(rows))
	for _, s := range rows {
		out[s.SourceID] = s
	}
	return out, nil
}
