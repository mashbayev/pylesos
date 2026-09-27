// Файл: internal/analyze/link.go
package analyze

import (
	"context"
	"database/sql"
	"strings"
	"time"

	docdomain "pylesos/internal/docforms/domain"
	pdomain "pylesos/internal/people/domain"
	"pylesos/internal/people/names"
	"pylesos/internal/validate"
)

// People — хранилище людей (people/repository).
type People interface {
	CreatePerson(ctx context.Context, p pdomain.Person) (int64, error)
	PersonByIIN(ctx context.Context, iin string) (pdomain.Person, bool, error)
	PersonsByKey(ctx context.Context, key string) ([]pdomain.Person, error)
	Keys(ctx context.Context) ([]pdomain.Person, error)
	Fill(ctx context.Context, id int64, p pdomain.Person) error
	ReplaceMentions(ctx context.Context, sourceID int64, msgID int, list []pdomain.Mention) error
	PersonsOfMessage(ctx context.Context, sourceID int64, msgID int) ([]int64, error)
	AddCandidate(ctx context.Context, a, b int64) error
}

// Linker привязывает упоминание к человеку (спецификация, раздел 4, шаг 3; П17).
type Linker struct {
	People People
	Now    func() time.Time
}

var none = sql.NullInt64{}

// Link: ИИН с верной контрольной цифрой → человек по ИИН; иначе точное ФИО, если такой человек один
// (никого — новый человек; несколько — не угадывать); нет имени — поля документа сообщения.
func (l *Linker) Link(ctx context.Context, f Found, docs []map[string]string) (sql.NullInt64, pdomain.Link, error) {
	doc := docFor(f, docs)
	name, iin := strings.TrimSpace(f.Name), validIIN(f.IIN)
	link := pdomain.LinkName
	if names.Key(name) == "" && doc != nil {
		name, link = doc[docdomain.FieldFullName], pdomain.LinkDocument
	}
	if iin == "" && doc != nil {
		iin = validIIN(doc[docdomain.FieldIIN])
	}
	if iin != "" {
		if link != pdomain.LinkDocument {
			link = pdomain.LinkIIN
		}
		id, err := l.byIIN(ctx, name, iin, doc)
		return sql.NullInt64{Int64: id, Valid: err == nil}, link, err
	}
	return l.byName(ctx, name, doc, link)
}

// docFor — документ, к которому относится упоминание: с тем же ИИН, иначе единственный в сообщении.
func docFor(f Found, docs []map[string]string) map[string]string {
	for _, d := range docs {
		if iin := validIIN(f.IIN); iin != "" && validIIN(d[docdomain.FieldIIN]) == iin {
			return d
		}
	}
	if len(docs) == 1 {
		return docs[0]
	}
	return nil
}

func validIIN(raw string) string {
	col, ok := docdomain.FieldColumn(docdomain.FieldIIN)
	if !ok || strings.TrimSpace(raw) == "" {
		return ""
	}
	if r := validate.Check(col, raw); len(r.Issues) == 0 {
		return r.Value
	}
	return ""
}

func (l *Linker) byIIN(ctx context.Context, name, iin string, doc map[string]string) (int64, error) {
	p, ok, err := l.People.PersonByIIN(ctx, iin)
	if err != nil {
		return 0, err
	}
	if ok {
		return p.ID, l.fill(ctx, p.ID, doc)
	}
	return l.create(ctx, name, sql.NullString{String: iin, Valid: true}, doc)
}

func (l *Linker) byName(ctx context.Context, name string, doc map[string]string, link pdomain.Link) (sql.NullInt64, pdomain.Link, error) {
	key := names.Key(name)
	if key == "" {
		return none, pdomain.LinkNone, nil
	}
	same, err := l.People.PersonsByKey(ctx, key)
	switch {
	case err != nil:
		return none, pdomain.LinkNone, err
	case len(same) > 1:
		return none, pdomain.LinkNone, nil // тёзки — привязывает пользователь
	case len(same) == 1:
		return sql.NullInt64{Int64: same[0].ID, Valid: true}, link, l.fill(ctx, same[0].ID, doc)
	}
	id, err := l.create(ctx, name, sql.NullString{}, doc)
	return sql.NullInt64{Int64: id, Valid: err == nil}, link, err
}

// create — новый человек и пары «возможно, один человек» с похожими именами.
// Тёзки с разными известными ИИН — точно разные люди, их не предлагаем.
func (l *Linker) create(ctx context.Context, name string, iin sql.NullString, doc map[string]string) (int64, error) {
	key := names.Key(name)
	p := pdomain.Person{FullName: strings.TrimSpace(name), NameKey: key, IIN: iin, CreatedAt: l.Now().Unix()}
	if doc != nil {
		p.DocNumber, p.BirthDate = doc[docdomain.FieldDocNumber], doc[docdomain.FieldBirthDate]
	}
	id, err := l.People.CreatePerson(ctx, p)
	if err != nil {
		return 0, err
	}
	return id, l.candidates(ctx, id, key, iin.Valid)
}

func (l *Linker) candidates(ctx context.Context, id int64, key string, hasIIN bool) error {
	all, err := l.People.Keys(ctx)
	if err != nil {
		return err
	}
	for _, other := range all {
		sameName := other.NameKey == key && !(hasIIN && other.IIN.Valid)
		if other.ID != id && (sameName || names.Similar(key, other.NameKey)) {
			if err := l.People.AddCandidate(ctx, id, other.ID); err != nil {
				return err
			}
		}
	}
	return nil
}

func (l *Linker) fill(ctx context.Context, id int64, doc map[string]string) error {
	if doc == nil {
		return nil
	}
	p := pdomain.Person{DocNumber: doc[docdomain.FieldDocNumber], BirthDate: doc[docdomain.FieldBirthDate]}
	if iin := validIIN(doc[docdomain.FieldIIN]); iin != "" {
		p.IIN = sql.NullString{String: iin, Valid: true}
	}
	return l.People.Fill(ctx, id, p)
}
