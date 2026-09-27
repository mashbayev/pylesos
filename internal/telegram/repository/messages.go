// Файл: internal/telegram/repository/messages.go
// Package repository — сообщения Telegram и сведения о вложениях.
package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jmoiron/sqlx"

	"pylesos/internal/telegram/domain"
)

// Кто принёс сообщение (П6).
const (
	ViaAccount = "account"
	ViaBot     = "bot"
)

type Repository interface {
	Save(ctx context.Context, sourceID int64, msgs []domain.Message, via string, at int64) error
	Bounds(ctx context.Context, sourceID int64) (minID, maxID int, err error)
	Counts(ctx context.Context) (map[int64]int, error)
	LiveIDsAfter(ctx context.Context, sourceID int64, afterID, limit int) ([]int, error)
	MarkDeleted(ctx context.Context, sourceID int64, ids []int) error
}

// Повторная загрузка не затирает сохранённое пустым и старой версией правки (02-database.md).
const upsertMessage = `INSERT INTO tg_messages (source_id, msg_id, date, author_id, author_name, text, reply_to,
	thread_top, fwd_from, edit_date, via, raw, fetched_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (source_id, msg_id) DO UPDATE SET
	author_id   = COALESCE(NULLIF(excluded.author_id, 0), tg_messages.author_id),
	author_name = COALESCE(NULLIF(excluded.author_name, ''), tg_messages.author_name),
	text        = CASE WHEN excluded.edit_date >= tg_messages.edit_date
	                   THEN COALESCE(NULLIF(excluded.text, ''), tg_messages.text) ELSE tg_messages.text END,
	edit_date   = MAX(excluded.edit_date, tg_messages.edit_date),
	raw         = COALESCE(NULLIF(excluded.raw, ''), tg_messages.raw),
	analyzed    = CASE WHEN excluded.edit_date > tg_messages.edit_date THEN 0 ELSE tg_messages.analyzed END,
	deleted     = 0,
	fetched_at  = excluded.fetched_at`

const insertMedia = `INSERT INTO tg_media (source_id, msg_id, idx, kind, file_id, mime_type, file_name, size)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT (source_id, msg_id, idx) DO NOTHING`

type SQLite struct{ db *sqlx.DB }

func NewSQLite(db *sqlx.DB) *SQLite { return &SQLite{db: db} }

// Save сохраняет порцию сообщений одной транзакцией.
func (r *SQLite) Save(ctx context.Context, sourceID int64, msgs []domain.Message, via string, at int64) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("начать запись сообщений: %w", err)
	}
	for _, m := range msgs {
		if err := saveOne(ctx, tx, sourceID, m, via, at); err != nil {
			return errors.Join(err, tx.Rollback())
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("сохранить сообщения: %w", err)
	}
	return nil
}

func saveOne(ctx context.Context, tx *sqlx.Tx, sourceID int64, m domain.Message, via string, at int64) error {
	if _, err := tx.ExecContext(ctx, upsertMessage, sourceID, m.ID, m.Date, m.AuthorID, m.AuthorName, m.Text,
		m.ReplyTo, m.ThreadTop, m.FwdFrom, m.EditDate, via, m.Raw, at); err != nil {
		return fmt.Errorf("сообщение %d: %w", m.ID, err)
	}
	for i, md := range m.Media {
		if _, err := tx.ExecContext(ctx, insertMedia, sourceID, m.ID, i, md.Kind, md.FileID, md.MimeType, md.FileName, md.Size); err != nil {
			return fmt.Errorf("вложение %d/%d: %w", m.ID, i, err)
		}
	}
	return nil
}

// Bounds — наименьший и наибольший номер сохранённых сообщений группы (0, 0 — сообщений нет).
func (r *SQLite) Bounds(ctx context.Context, sourceID int64) (int, int, error) {
	var b struct {
		Min int `db:"min_id"`
		Max int `db:"max_id"`
	}
	if err := r.db.GetContext(ctx, &b, `SELECT COALESCE(MIN(msg_id), 0) AS min_id, COALESCE(MAX(msg_id), 0) AS max_id
		FROM tg_messages WHERE source_id = ?`, sourceID); err != nil {
		return 0, 0, fmt.Errorf("границы сообщений: %w", err)
	}
	return b.Min, b.Max, nil
}

// Counts — число неудалённых сообщений по источникам.
func (r *SQLite) Counts(ctx context.Context) (map[int64]int, error) {
	var rows []struct {
		SourceID int64 `db:"source_id"`
		N        int   `db:"n"`
	}
	if err := r.db.SelectContext(ctx, &rows, `SELECT source_id, COUNT(*) AS n FROM tg_messages
		WHERE deleted = 0 GROUP BY source_id`); err != nil {
		return nil, fmt.Errorf("число сообщений: %w", err)
	}
	out := make(map[int64]int, len(rows))
	for _, x := range rows {
		out[x.SourceID] = x.N
	}
	return out, nil
}

func (r *SQLite) LiveIDsAfter(ctx context.Context, sourceID int64, afterID, limit int) ([]int, error) {
	ids := []int{}
	if err := r.db.SelectContext(ctx, &ids, `SELECT msg_id FROM tg_messages
		WHERE source_id = ? AND deleted = 0 AND msg_id > ? ORDER BY msg_id LIMIT ?`, sourceID, afterID, limit); err != nil {
		return nil, fmt.Errorf("номера сообщений: %w", err)
	}
	return ids, nil
}

func (r *SQLite) MarkDeleted(ctx context.Context, sourceID int64, ids []int) error {
	if len(ids) == 0 {
		return nil
	}
	query, args, err := sqlx.In(`UPDATE tg_messages SET deleted = 1 WHERE source_id = ? AND msg_id IN (?)`, sourceID, ids)
	if err != nil {
		return fmt.Errorf("запрос удалённых: %w", err)
	}
	if _, err := r.db.ExecContext(ctx, r.db.Rebind(query), args...); err != nil {
		return fmt.Errorf("пометить удалённые: %w", err)
	}
	return nil
}
