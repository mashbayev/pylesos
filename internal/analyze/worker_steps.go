// Файл: internal/analyze/worker_steps.go
package analyze

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	pdomain "pylesos/internal/people/domain"
	"pylesos/internal/people/names"
	tgdomain "pylesos/internal/telegram/domain"
	tgrepo "pylesos/internal/telegram/repository"
	"pylesos/internal/texts"
)

// docsBatch распознаёт порцию вложений; false — вложений больше нет.
func (r *run) docsBatch(ctx context.Context) (bool, error) {
	refs, err := r.w.d.Messages.PendingMedia(ctx, r.source, batchDocs)
	if err != nil || len(refs) == 0 {
		return false, err
	}
	for _, ref := range refs {
		if err := r.readMedia(ctx, ref); err != nil {
			return false, err
		}
		r.docs++
		if err := r.p.Save(ctx, "{}", fmt.Sprintf(texts.AnalyzeDocs, r.docs)); err != nil {
			return false, err
		}
	}
	return true, nil
}

// readMedia скачивает вложение, распознаёт и сохраняет текст и поля; сам файл не сохраняется (П12).
func (r *run) readMedia(ctx context.Context, ref tgrepo.MediaRef) error {
	files, err := r.w.d.Gateway.Media(ctx, r.peer, ref.MsgID)
	if err != nil {
		return err
	}
	if len(files) == 0 { // сообщение удалено или вложение не файл
		return r.w.d.Messages.SaveRecognition(ctx, r.source, ref.MsgID, ref.Idx, "", "{}", r.model)
	}
	text, fields, err := r.w.docs.Read(ctx, r.model, files[0])
	if errors.Is(err, errUnreadable) {
		return r.w.d.Messages.SaveRecognition(ctx, r.source, ref.MsgID, ref.Idx, "", "{}", r.model)
	}
	if err != nil {
		return err
	}
	data, err := json.Marshal(fields)
	if err != nil {
		return fmt.Errorf("поля документа: %w", err)
	}
	return r.w.d.Messages.SaveRecognition(ctx, r.source, ref.MsgID, ref.Idx, text, string(data), r.model)
}

// msgsBatch разбирает порцию сообщений; false — неразобранных больше нет.
func (r *run) msgsBatch(ctx context.Context) (bool, error) {
	msgs, err := r.w.d.Messages.PendingAnalysis(ctx, r.source, batchMsgs)
	if err != nil || len(msgs) == 0 {
		return false, err
	}
	for _, m := range msgs {
		if err := r.analyze(ctx, m); err != nil {
			return false, err
		}
		r.msgs++
		if err := r.p.Save(ctx, "{}", fmt.Sprintf(texts.AnalyzeMsgs, r.msgs)); err != nil {
			return false, err
		}
	}
	return true, nil
}

// analyze — люди сообщения и их привязка; ответ без людей — комментарий к людям исходного сообщения.
func (r *run) analyze(ctx context.Context, m tgdomain.Message) error {
	docs, err := r.w.d.Messages.MediaFields(ctx, r.source, m.ID)
	if err != nil {
		return err
	}
	reply, replyPersons, err := r.replyContext(ctx, m)
	if err != nil {
		return err
	}
	var found []Found
	if strings.TrimSpace(m.Text) != "" || len(docs) > 0 {
		found, err = r.w.finder.Find(ctx, r.model, m, docs, reply)
		if errors.Is(err, ErrBadAnswer) {
			return r.w.d.Messages.MarkAnalyzed(ctx, r.source, m.ID, texts.AnalyzeBadAnswer)
		}
		if err != nil {
			return err
		}
	}
	if len(found) == 0 && len(docs) > 0 {
		found = []Found{{Role: string(pdomain.RoleRecord)}} // документ без слов — человек из документа
	}
	mentions, err := r.link(ctx, m, found, docs, replyPersons)
	if err != nil {
		return err
	}
	if err := r.w.d.People.ReplaceMentions(ctx, r.source, m.ID, mentions); err != nil {
		return err
	}
	return r.w.d.Messages.MarkAnalyzed(ctx, r.source, m.ID, "")
}

func (r *run) replyContext(ctx context.Context, m tgdomain.Message) (string, []int64, error) {
	if m.ReplyTo == 0 {
		return "", nil, nil
	}
	orig, ok, err := r.w.d.Messages.Message(ctx, r.source, m.ReplyTo)
	if err != nil || !ok {
		return "", nil, err
	}
	persons, err := r.w.d.People.PersonsOfMessage(ctx, r.source, m.ReplyTo)
	return orig.Text, persons, err
}

func (r *run) link(ctx context.Context, m tgdomain.Message, found []Found, docs []map[string]string, replyPersons []int64) ([]pdomain.Mention, error) {
	mentions := []pdomain.Mention{}
	for _, f := range found {
		id, link, err := r.w.linker.Link(ctx, f, docs)
		if err != nil {
			return nil, err
		}
		if link == pdomain.LinkNone && len(replyPersons) > 0 {
			if id, err = r.namesakeInReply(ctx, f.Name, replyPersons); err != nil {
				return nil, err
			}
			if id.Valid {
				link = pdomain.LinkReply
			}
		}
		mentions = append(mentions, pdomain.Mention{SourceID: r.source, MsgID: m.ID, PersonID: id,
			Role: pdomain.Role(f.Role), NameAsWritten: strings.TrimSpace(f.Name), Link: link})
	}
	if len(found) == 0 {
		for _, pid := range replyPersons {
			mentions = append(mentions, pdomain.Mention{SourceID: r.source, MsgID: m.ID, Role: pdomain.RoleComment,
				PersonID: sql.NullInt64{Int64: pid, Valid: true}, Link: pdomain.LinkReply})
		}
	}
	return mentions, nil
}

// namesakeInReply — из нескольких тёзок единственный, о ком исходное сообщение (ответ говорит о нём).
func (r *run) namesakeInReply(ctx context.Context, name string, replyPersons []int64) (sql.NullInt64, error) {
	key := names.Key(name)
	if key == "" {
		return sql.NullInt64{}, nil
	}
	same, err := r.w.d.People.PersonsByKey(ctx, key)
	if err != nil {
		return sql.NullInt64{}, err
	}
	var hit sql.NullInt64
	for _, p := range same {
		if slices.Contains(replyPersons, p.ID) {
			if hit.Valid {
				return sql.NullInt64{}, nil // и в исходном сообщении тёзки — не угадывать
			}
			hit = sql.NullInt64{Int64: p.ID, Valid: true}
		}
	}
	return hit, nil
}
