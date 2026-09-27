// Файл: internal/table/repository/sqlite_test.go
package repository

import (
	"context"
	"database/sql"
	"testing"

	pdomain "pylesos/internal/people/domain"
	prepo "pylesos/internal/people/repository"
	"pylesos/internal/store/storetest"
	"pylesos/internal/table/domain"
	"pylesos/internal/tables"
	tgdomain "pylesos/internal/telegram/domain"
	tgrepo "pylesos/internal/telegram/repository"
)

func TestConfigRowsPeopleDocs(t *testing.T) {
	ctx := context.Background()
	db := storetest.Open(t)
	src := storetest.AddSource(t, db, "Группа")
	msgs := tgrepo.NewSQLite(db)
	if err := msgs.Save(ctx, src, []tgdomain.Message{
		{ID: 1, Text: "Тестов Иван ЧС", Media: []tgdomain.Media{{Kind: tgdomain.MediaPhoto, FileID: 7}}},
		{ID: 2, Text: "Петров Пётр есть инфо?"},
	}, tgrepo.ViaAccount, 1); err != nil {
		t.Fatal(err)
	}
	if err := msgs.SaveRecognition(ctx, src, 1, 0, "УДОСТОВЕРЕНИЕ ТЕСТОВ ИВАН", "{}", "m"); err != nil {
		t.Fatal(err)
	}
	people := prepo.NewSQLite(db)
	ivan, err := people.CreatePerson(ctx, pdomain.Person{FullName: "Тестов Иван", NameKey: "иван тестов", CreatedAt: 1})
	if err != nil {
		t.Fatal(err)
	}
	petr, err := people.CreatePerson(ctx, pdomain.Person{FullName: "Петров Пётр", NameKey: "петр петров", CreatedAt: 1})
	if err != nil {
		t.Fatal(err)
	}
	link := func(msg int, id int64, role pdomain.Role) {
		m := pdomain.Mention{SourceID: src, MsgID: msg, PersonID: nullID(id), Role: role, Link: pdomain.LinkName}
		if err := people.ReplaceMentions(ctx, src, msg, []pdomain.Mention{m}); err != nil {
			t.Fatal(err)
		}
	}
	link(1, ivan, pdomain.RoleRecord)
	link(2, petr, pdomain.RoleQuery)

	r := NewSQLite(db)
	if _, ok, err := r.Config(ctx); ok || err != nil {
		t.Fatalf("пустая настройка: %v %v", ok, err)
	}
	cfg := domain.Config{Columns: []tables.Column{{Name: "ИИН", Type: tables.TypeIIN}}, IncludeQueries: true}
	if err := r.SaveConfig(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	if got, ok, err := r.Config(ctx); !ok || err != nil || got.Columns[0].Name != "ИИН" || !got.IncludeQueries {
		t.Fatalf("настройка: %+v %v %v", got, ok, err)
	}
	if list, err := r.People(ctx, src, false); err != nil || len(list) != 1 || list[0].ID != ivan {
		t.Fatalf("без запросов: %+v %v", list, err)
	}
	if list, err := r.People(ctx, src, true); err != nil || len(list) != 2 {
		t.Fatalf("с запросами: %+v %v", list, err)
	}
	if docs, err := r.DocTexts(ctx, ivan); err != nil || len(docs) != 1 || docs[0] != "УДОСТОВЕРЕНИЕ ТЕСТОВ ИВАН" {
		t.Fatalf("документы: %v %v", docs, err)
	}
	row := tables.Row{Cells: map[string]tables.Cell{"ИИН": {Value: "900101300007"}}}
	if err := r.SaveRow(ctx, ivan, "h1", row); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := r.Row(ctx, ivan, "h2"); ok || err != nil {
		t.Fatalf("другой отпечаток: %v %v", ok, err)
	}
	if got, ok, err := r.Row(ctx, ivan, "h1"); !ok || err != nil || got.Cells["ИИН"].Value != "900101300007" {
		t.Fatalf("строка: %+v %v %v", got, ok, err)
	}
}

func nullID(id int64) sql.NullInt64 { return sql.NullInt64{Int64: id, Valid: true} }
