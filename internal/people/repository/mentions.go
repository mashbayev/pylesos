// Файл: internal/people/repository/mentions.go
package repository

import (
	"context"
	"errors"
	"fmt"

	"pylesos/internal/people/domain"
)

// ReplaceMentions заменяет упоминания сообщения (повторный разбор после правки — без дублей).
func (r *SQLite) ReplaceMentions(ctx context.Context, sourceID int64, msgID int, list []domain.Mention) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("начать запись упоминаний: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM mentions WHERE source_id = ? AND msg_id = ?`, sourceID, msgID); err != nil {
		return errors.Join(fmt.Errorf("убрать упоминания %d: %w", msgID, err), tx.Rollback())
	}
	for _, m := range list {
		if _, err := tx.NamedExecContext(ctx, `INSERT INTO mentions (source_id, msg_id, person_id, role, name_as_written, link)
			VALUES (:source_id, :msg_id, :person_id, :role, :name_as_written, :link)`, m); err != nil {
			return errors.Join(fmt.Errorf("записать упоминание %d: %w", msgID, err), tx.Rollback())
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("сохранить упоминания %d: %w", msgID, err)
	}
	return nil
}

// PersonsOfMessage — люди, привязанные к сообщению (для цепочки ответов).
func (r *SQLite) PersonsOfMessage(ctx context.Context, sourceID int64, msgID int) ([]int64, error) {
	ids := []int64{}
	if err := r.db.SelectContext(ctx, &ids, `SELECT DISTINCT person_id FROM mentions
		WHERE source_id = ? AND msg_id = ? AND person_id IS NOT NULL ORDER BY person_id`, sourceID, msgID); err != nil {
		return nil, fmt.Errorf("люди сообщения %d: %w", msgID, err)
	}
	return ids, nil
}

// Unlinked — упоминания без человека (для ручной привязки).
func (r *SQLite) Unlinked(ctx context.Context) ([]domain.Mention, error) {
	list := []domain.Mention{}
	if err := r.db.SelectContext(ctx, &list, `SELECT id, source_id, msg_id, person_id, role, name_as_written, link
		FROM mentions WHERE person_id IS NULL ORDER BY source_id, msg_id`); err != nil {
		return nil, fmt.Errorf("непривязанные упоминания: %w", err)
	}
	return list, nil
}

func (r *SQLite) LinkMention(ctx context.Context, mentionID, personID int64) error {
	res, err := r.db.ExecContext(ctx, `UPDATE mentions SET person_id = ?, link = 'manual' WHERE id = ?`, personID, mentionID)
	if err != nil {
		return fmt.Errorf("привязать упоминание %d: %w", mentionID, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("привязать упоминание %d: %w", mentionID, err)
	}
	if n == 0 {
		return domain.ErrNotFound
	}
	return nil
}
