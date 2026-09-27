// Файл: internal/people/repository/repo_test.go
package repository

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"pylesos/internal/people/domain"
	"pylesos/internal/store/storetest"
	tgdomain "pylesos/internal/telegram/domain"
	tgrepo "pylesos/internal/telegram/repository"
)

type env struct {
	repo *SQLite
	src  int64
}

func newEnv(t *testing.T) env {
	t.Helper()
	db := storetest.Open(t)
	src := storetest.AddSource(t, db, "Группа")
	msgs := []tgdomain.Message{{ID: 1, Text: "а"}, {ID: 2, Text: "б"}, {ID: 3, Text: "в"}}
	if err := tgrepo.NewSQLite(db).Save(context.Background(), src, msgs, tgrepo.ViaAccount, 1); err != nil {
		t.Fatal(err)
	}
	return env{repo: NewSQLite(db), src: src}
}

func person(name, key, iin string) domain.Person {
	p := domain.Person{FullName: name, NameKey: key, CreatedAt: 1}
	if iin != "" {
		p.IIN = sql.NullString{String: iin, Valid: true}
	}
	return p
}

func TestPersonsLookupAndFill(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	a, err := e.repo.CreatePerson(ctx, person("Тестов Иван", "иван тестов", "900101300011"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.repo.CreatePerson(ctx, person("Тестов Иван", "иван тестов", "")); err != nil {
		t.Fatal(err)
	}
	if p, ok, err := e.repo.PersonByIIN(ctx, "900101300011"); err != nil || !ok || p.ID != a {
		t.Fatalf("по ИИН: %+v %v %v", p, ok, err)
	}
	if list, err := e.repo.PersonsByKey(ctx, "иван тестов"); err != nil || len(list) != 2 {
		t.Fatalf("по ключу: %d %v", len(list), err)
	}
	if err := e.repo.Fill(ctx, a, domain.Person{DocNumber: "N123", BirthDate: "01.01.1990"}); err != nil {
		t.Fatal(err)
	}
	if err := e.repo.Fill(ctx, a, domain.Person{DocNumber: ""}); err != nil { // пустое не затирает
		t.Fatal(err)
	}
	if p, err := e.repo.Person(ctx, a); err != nil || p.DocNumber != "N123" || p.BirthDate != "01.01.1990" {
		t.Fatalf("заполнение: %+v %v", p, err)
	}
}

func TestMentionsReplaceAndReplyChain(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	a, err := e.repo.CreatePerson(ctx, person("Тестов Иван", "иван тестов", ""))
	if err != nil {
		t.Fatal(err)
	}
	first := []domain.Mention{{SourceID: e.src, MsgID: 1, PersonID: sql.NullInt64{Int64: a, Valid: true}, Role: domain.RoleRecord, Link: domain.LinkName}}
	if err := e.repo.ReplaceMentions(ctx, e.src, 1, first); err != nil {
		t.Fatal(err)
	}
	if err := e.repo.ReplaceMentions(ctx, e.src, 1, first); err != nil { // повторный разбор — без дублей
		t.Fatal(err)
	}
	ids, err := e.repo.PersonsOfMessage(ctx, e.src, 1)
	if err != nil || len(ids) != 1 || ids[0] != a {
		t.Fatalf("люди сообщения: %v %v", ids, err)
	}
	loose := []domain.Mention{{SourceID: e.src, MsgID: 2, Role: domain.RoleRecord, NameAsWritten: "Тестов И.", Link: domain.LinkNone}}
	if err := e.repo.ReplaceMentions(ctx, e.src, 2, loose); err != nil {
		t.Fatal(err)
	}
	unlinked, err := e.repo.Unlinked(ctx)
	if err != nil || len(unlinked) != 1 || unlinked[0].NameAsWritten != "Тестов И." {
		t.Fatalf("не привязано: %+v %v", unlinked, err)
	}
	if err := e.repo.LinkMention(ctx, unlinked[0].ID, a); err != nil {
		t.Fatal(err)
	}
	if unlinked, err := e.repo.Unlinked(ctx); err != nil || len(unlinked) != 0 {
		t.Fatalf("после ручной привязки: %+v %v", unlinked, err)
	}
}

func TestCandidatesRejectAndMerge(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	a, err := e.repo.CreatePerson(ctx, person("Тестов Иван", "иван тестов", ""))
	if err != nil {
		t.Fatal(err)
	}
	b, err := e.repo.CreatePerson(ctx, person("Тестов Иван Петрович", "иван петрович тестов", "900101300011"))
	if err != nil {
		t.Fatal(err)
	}
	c, err := e.repo.CreatePerson(ctx, person("Тестов Иваан", "иваан тестов", ""))
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range [][2]int64{{b, a}, {a, b}, {a, c}} { // порядок пары не важен, дубль не создаётся
		if err := e.repo.AddCandidate(ctx, p[0], p[1]); err != nil {
			t.Fatal(err)
		}
	}
	if list, err := e.repo.Candidates(ctx); err != nil || len(list) != 2 {
		t.Fatalf("кандидаты: %+v %v", list, err)
	}
	if err := e.repo.Reject(ctx, a, c); err != nil {
		t.Fatal(err)
	}
	if err := e.repo.AddCandidate(ctx, c, a); err != nil { // отклонённая пара не возвращается
		t.Fatal(err)
	}
	mb := []domain.Mention{{SourceID: e.src, MsgID: 3, PersonID: sql.NullInt64{Int64: b, Valid: true}, Role: domain.RoleComment, Link: domain.LinkIIN}}
	if err := e.repo.ReplaceMentions(ctx, e.src, 3, mb); err != nil {
		t.Fatal(err)
	}
	if err := e.repo.Merge(ctx, a, b); err != nil {
		t.Fatal(err)
	}
	if _, err := e.repo.Person(ctx, b); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("второй не удалён: %v", err)
	}
	if p, err := e.repo.Person(ctx, a); err != nil || p.IIN.String != "900101300011" {
		t.Fatalf("ИИН не перенесён: %+v %v", p, err)
	}
	if ids, err := e.repo.PersonsOfMessage(ctx, e.src, 3); err != nil || len(ids) != 1 || ids[0] != a {
		t.Fatalf("упоминания не перенесены: %v %v", ids, err)
	}
	if list, err := e.repo.Candidates(ctx); err != nil || len(list) != 0 {
		t.Fatalf("после склейки кандидатов: %+v %v", list, err)
	}
}
