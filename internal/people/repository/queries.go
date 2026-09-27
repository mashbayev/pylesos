// Файл: internal/people/repository/queries.go
package repository

import (
	"context"
	"fmt"

	"pylesos/internal/people/domain"
	"pylesos/internal/people/names"
)

// Search — люди со сводкой упоминаний; query ищется в ФИО (в т.ч. нормализованном) и ИИН. Пусто — все.
func (r *SQLite) Search(ctx context.Context, query string, limit int) ([]domain.Summary, error) {
	like := "%" + query + "%"
	keyLike := "%" + names.Key(query) + "%"
	if names.Key(query) == "" {
		keyLike = "" // в запросе нет букв (например, ИИН) — по ключу ФИО не ищем
	}
	list := []domain.Summary{}
	if err := r.db.SelectContext(ctx, &list, `SELECT p.id, p.full_name, p.name_key, p.iin, p.doc_number, p.birth_date, p.created_at,
		COALESCE(SUM(m.role = 'record'), 0) AS records,
		COALESCE(SUM(m.role = 'query'), 0) AS queries,
		COALESCE(SUM(m.role = 'comment'), 0) AS comments,
		COALESCE(GROUP_CONCAT(DISTINCT s.title), '') AS groups
		FROM persons p
		LEFT JOIN mentions m ON m.person_id = p.id
		LEFT JOIN sources s ON s.id = m.source_id
		WHERE ? = '' OR p.full_name LIKE ? OR p.iin LIKE ? OR (? <> '' AND p.name_key LIKE ?)
		GROUP BY p.id ORDER BY p.full_name, p.id LIMIT ?`, query, like, like, keyLike, keyLike, limit); err != nil {
		return nil, fmt.Errorf("поиск людей: %w", err)
	}
	return list, nil
}

const viewCols = `m.id, m.source_id, m.msg_id, m.person_id, m.role, m.name_as_written, m.link,
	t.date, t.author_name, t.text, s.title AS source_title, s.config AS source_config`

const viewJoin = ` FROM mentions m
	JOIN tg_messages t ON t.source_id = m.source_id AND t.msg_id = m.msg_id
	JOIN sources s ON s.id = m.source_id`

// PersonMentions — упоминания человека с текстами сообщений, по времени.
func (r *SQLite) PersonMentions(ctx context.Context, personID int64) ([]domain.MentionView, error) {
	list := []domain.MentionView{}
	if err := r.db.SelectContext(ctx, &list, `SELECT `+viewCols+viewJoin+`
		WHERE m.person_id = ? ORDER BY t.date, m.msg_id`, personID); err != nil {
		return nil, fmt.Errorf("упоминания человека %d: %w", personID, err)
	}
	return list, nil
}

// UnlinkedViews — упоминания без человека с текстами сообщений.
func (r *SQLite) UnlinkedViews(ctx context.Context) ([]domain.MentionView, error) {
	list := []domain.MentionView{}
	if err := r.db.SelectContext(ctx, &list, `SELECT `+viewCols+viewJoin+`
		WHERE m.person_id IS NULL ORDER BY t.date, m.msg_id`); err != nil {
		return nil, fmt.Errorf("непривязанные упоминания: %w", err)
	}
	return list, nil
}
