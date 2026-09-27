// Файл: internal/telegram/repository/analysis.go
package repository

import (
	"context"
	"encoding/json"
	"fmt"

	"pylesos/internal/telegram/domain"
)

// messageRow — сообщение для разбора (без медиа и сырого JSON).
type messageRow struct {
	ID         int    `db:"msg_id"`
	Date       int64  `db:"date"`
	AuthorID   int64  `db:"author_id"`
	AuthorName string `db:"author_name"`
	Text       string `db:"text"`
	ReplyTo    int    `db:"reply_to"`
	ThreadTop  int    `db:"thread_top"`
	FwdFrom    string `db:"fwd_from"`
	EditDate   int64  `db:"edit_date"`
}

const messageCols = `msg_id, date, author_id, author_name, text, reply_to, thread_top, fwd_from, edit_date`

func (r messageRow) message() domain.Message {
	return domain.Message{ID: r.ID, Date: r.Date, AuthorID: r.AuthorID, AuthorName: r.AuthorName, Text: r.Text,
		ReplyTo: r.ReplyTo, ThreadTop: r.ThreadTop, FwdFrom: r.FwdFrom, EditDate: r.EditDate}
}

// PendingAnalysis — неразобранные сообщения группы по порядку номеров.
func (r *SQLite) PendingAnalysis(ctx context.Context, sourceID int64, limit int) ([]domain.Message, error) {
	var rows []messageRow
	if err := r.db.SelectContext(ctx, &rows, `SELECT `+messageCols+` FROM tg_messages
		WHERE source_id = ? AND deleted = 0 AND analyzed = 0 ORDER BY msg_id LIMIT ?`, sourceID, limit); err != nil {
		return nil, fmt.Errorf("сообщения для разбора: %w", err)
	}
	out := make([]domain.Message, 0, len(rows))
	for _, x := range rows {
		out = append(out, x.message())
	}
	return out, nil
}

// Message — одно сообщение группы (для контекста ответа); ok=false — нет такого.
func (r *SQLite) Message(ctx context.Context, sourceID int64, msgID int) (domain.Message, bool, error) {
	var rows []messageRow
	if err := r.db.SelectContext(ctx, &rows, `SELECT `+messageCols+` FROM tg_messages
		WHERE source_id = ? AND msg_id = ?`, sourceID, msgID); err != nil {
		return domain.Message{}, false, fmt.Errorf("сообщение %d: %w", msgID, err)
	}
	if len(rows) == 0 {
		return domain.Message{}, false, nil
	}
	return rows[0].message(), true, nil
}

// MarkAnalyzed — сообщение разобрано; errText != "" — разбор не удался («не разобрано: ошибка модели»).
func (r *SQLite) MarkAnalyzed(ctx context.Context, sourceID int64, msgID int, errText string) error {
	status := 1
	if errText != "" {
		status = 2
	}
	if _, err := r.db.ExecContext(ctx, `UPDATE tg_messages SET analyzed = ?, analyze_error = ?
		WHERE source_id = ? AND msg_id = ?`, status, errText, sourceID, msgID); err != nil {
		return fmt.Errorf("отметить разбор сообщения %d: %w", msgID, err)
	}
	return nil
}

// MediaRef — вложение, ждущее распознавания.
type MediaRef struct {
	MsgID    int    `db:"msg_id"`
	Idx      int    `db:"idx"`
	Kind     string `db:"kind"`
	MimeType string `db:"mime_type"`
	FileName string `db:"file_name"`
}

func (r *SQLite) PendingMedia(ctx context.Context, sourceID int64, limit int) ([]MediaRef, error) {
	list := []MediaRef{}
	if err := r.db.SelectContext(ctx, &list, `SELECT msg_id, idx, kind, mime_type, file_name FROM tg_media
		WHERE source_id = ? AND status = 'pending' ORDER BY msg_id, idx LIMIT ?`, sourceID, limit); err != nil {
		return nil, fmt.Errorf("вложения для распознавания: %w", err)
	}
	return list, nil
}

// SaveRecognition — итог распознавания вложения: текст, опознавательные поля (JSON) и модель; файл не хранится (П12).
func (r *SQLite) SaveRecognition(ctx context.Context, sourceID int64, msgID, idx int, text, fields, model string) error {
	if _, err := r.db.ExecContext(ctx, `UPDATE tg_media SET status = 'recognized', recognized_text = ?, fields = ?, model = ?
		WHERE source_id = ? AND msg_id = ? AND idx = ?`, text, fields, model, sourceID, msgID, idx); err != nil {
		return fmt.Errorf("распознавание вложения %d/%d: %w", msgID, idx, err)
	}
	return nil
}

// MediaFields — опознавательные поля распознанных документов сообщения.
func (r *SQLite) MediaFields(ctx context.Context, sourceID int64, msgID int) ([]map[string]string, error) {
	var raw []string
	if err := r.db.SelectContext(ctx, &raw, `SELECT fields FROM tg_media
		WHERE source_id = ? AND msg_id = ? AND status = 'recognized' ORDER BY idx`, sourceID, msgID); err != nil {
		return nil, fmt.Errorf("поля документов %d: %w", msgID, err)
	}
	out := make([]map[string]string, 0, len(raw))
	for _, s := range raw {
		f := map[string]string{}
		if err := json.Unmarshal([]byte(s), &f); err != nil {
			return nil, fmt.Errorf("поля документов %d: %w", msgID, err)
		}
		if len(f) > 0 {
			out = append(out, f)
		}
	}
	return out, nil
}
