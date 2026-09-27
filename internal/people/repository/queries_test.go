// Файл: internal/people/repository/queries_test.go
package repository

import (
	"context"
	"database/sql"
	"testing"

	"pylesos/internal/people/domain"
)

func TestSearchAndViews(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	a, err := e.repo.CreatePerson(ctx, person("Тестов Иван", "иван тестов", "900101300007"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.repo.CreatePerson(ctx, person("Примеров Пётр", "петр примеров", "")); err != nil {
		t.Fatal(err)
	}
	pid := sql.NullInt64{Int64: a, Valid: true}
	if err := e.repo.ReplaceMentions(ctx, e.src, 1, []domain.Mention{{SourceID: e.src, MsgID: 1, PersonID: pid, Role: domain.RoleRecord, Link: domain.LinkName}}); err != nil {
		t.Fatal(err)
	}
	if err := e.repo.ReplaceMentions(ctx, e.src, 2, []domain.Mention{{SourceID: e.src, MsgID: 2, PersonID: pid, Role: domain.RoleComment, Link: domain.LinkReply}}); err != nil {
		t.Fatal(err)
	}
	if err := e.repo.ReplaceMentions(ctx, e.src, 3, []domain.Mention{{SourceID: e.src, MsgID: 3, Role: domain.RoleRecord, NameAsWritten: "Тестов", Link: domain.LinkNone}}); err != nil {
		t.Fatal(err)
	}
	all, err := e.repo.Search(ctx, "", 50)
	if err != nil || len(all) != 2 {
		t.Fatalf("все: %+v %v", all, err)
	}
	found, err := e.repo.Search(ctx, "иван", 50)
	if err != nil || len(found) != 1 || found[0].Records != 1 || found[0].Comments != 1 || found[0].Groups != "Группа" {
		t.Fatalf("поиск: %+v %v", found, err)
	}
	if byIIN, err := e.repo.Search(ctx, "9001013", 50); err != nil || len(byIIN) != 1 {
		t.Fatalf("поиск по ИИН: %+v %v", byIIN, err)
	}
	views, err := e.repo.PersonMentions(ctx, a)
	if err != nil || len(views) != 2 || views[0].Text != "а" || views[0].SourceTitle != "Группа" {
		t.Fatalf("упоминания человека: %+v %v", views, err)
	}
	loose, err := e.repo.UnlinkedViews(ctx)
	if err != nil || len(loose) != 1 || loose[0].Text != "в" {
		t.Fatalf("не привязано: %+v %v", loose, err)
	}
}
