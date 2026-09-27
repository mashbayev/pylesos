// Файл: internal/table/service/export_test.go
package service

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"pylesos/internal/analyze"
	jobsdomain "pylesos/internal/jobs/domain"
	jobsservice "pylesos/internal/jobs/service"
	"pylesos/internal/llm"
	pdomain "pylesos/internal/people/domain"
	prepo "pylesos/internal/people/repository"
	settingsdomain "pylesos/internal/settings/domain"
	sourcesrepo "pylesos/internal/sources/repository"
	"pylesos/internal/store/storetest"
	"pylesos/internal/table/domain"
	"pylesos/internal/table/repository"
	"pylesos/internal/tables"
	tgdomain "pylesos/internal/telegram/domain"
	tgrepo "pylesos/internal/telegram/repository"
	"pylesos/internal/texts"
)

type fakeLLM struct {
	calls int
	texts []string
}

func (f *fakeLLM) Respond(_ context.Context, r llm.Request) (string, error) {
	f.calls++
	f.texts = append(f.texts, r.Text)
	return `{"rows":[{"ИИН":{"value":"900101300007","confidence":"high"},"Телефон":{"value":"","confidence":"high"}}]}`, nil
}

type fakeModels struct{ err error }

func (f fakeModels) Pick(context.Context) (string, error) { return "m", f.err }

type fakeSettings struct{}

func (fakeSettings) Get(context.Context) (settingsdomain.Settings, error) {
	return settingsdomain.Settings{ExportsDir: "/выгрузки"}, nil
}

type noProgress struct{}

func (noProgress) Save(context.Context, string, string) error { return nil }

type written struct {
	dir  string
	tpl  tables.Template
	rows []tables.Row
}

func setup(t *testing.T) (*Exporter, *fakeLLM, *written, int64) {
	t.Helper()
	ctx := context.Background()
	db := storetest.Open(t)
	src := storetest.AddSource(t, db, "ЧС Тест")
	if _, err := db.Exec(`UPDATE sources SET config = '{"type":"channel","id":55,"access_hash":1}' WHERE id = ?`, src); err != nil {
		t.Fatal(err)
	}
	msgs := tgrepo.NewSQLite(db)
	date := time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC).Unix()
	if err := msgs.Save(ctx, src, []tgdomain.Message{
		{ID: 4, Date: date, AuthorName: "Админ", Text: "Тестов Иван — ЧС, не вернул авто", Media: []tgdomain.Media{{Kind: tgdomain.MediaPhoto, FileID: 1}}},
		{ID: 5, Date: date + 60, AuthorName: "Марат", Text: "подтверждаю"},
	}, tgrepo.ViaAccount, 1); err != nil {
		t.Fatal(err)
	}
	if err := msgs.SaveRecognition(ctx, src, 4, 0, "УДОСТОВЕРЕНИЕ ЛИЧНОСТИ ТЕСТОВ ИВАН", "{}", "m"); err != nil {
		t.Fatal(err)
	}
	people := prepo.NewSQLite(db)
	id, err := people.CreatePerson(ctx, pdomain.Person{FullName: "Тестов Иван", NameKey: "иван тестов", CreatedAt: 1})
	if err != nil {
		t.Fatal(err)
	}
	for msg, role := range map[int]pdomain.Role{4: pdomain.RoleRecord, 5: pdomain.RoleComment} {
		m := pdomain.Mention{SourceID: src, MsgID: msg, PersonID: sql.NullInt64{Int64: id, Valid: true}, Role: role, Link: pdomain.LinkName}
		if err := people.ReplaceMentions(ctx, src, msg, []pdomain.Mention{m}); err != nil {
			t.Fatal(err)
		}
	}
	store := repository.NewSQLite(db)
	cfg := NewConfig(store)
	if err := cfg.Save(ctx, domain.Config{Columns: []tables.Column{{Name: "ИИН", Type: tables.TypeIIN}, {Name: "Телефон", Type: tables.TypePhone}}}); err != nil {
		t.Fatal(err)
	}
	model, out := &fakeLLM{}, &written{}
	e := NewExporter(ExportDeps{Config: cfg, Store: store, Mentions: people, Sources: sourcesrepo.NewSQLite(db), Settings: fakeSettings{},
		LLM: model, Models: fakeModels{}, Loc: time.UTC,
		Write: func(dir string, tpl tables.Template, rows []tables.Row) (string, error) {
			*out = written{dir: dir, tpl: tpl, rows: rows}
			return dir + "/x.xlsx", nil
		}})
	return e, model, out, src
}

func TestExportFillsColumnsAndComments(t *testing.T) {
	e, model, out, src := setup(t)
	if err := e.Run(context.Background(), jobsdomain.Job{SourceID: src}, noProgress{}); err != nil {
		t.Fatal(err)
	}
	if model.calls != 1 || !strings.Contains(model.texts[0], "УДОСТОВЕРЕНИЕ ЛИЧНОСТИ ТЕСТОВ ИВАН") || !strings.Contains(model.texts[0], "подтверждаю") {
		t.Fatalf("вход модели: %d %q", model.calls, model.texts)
	}
	if out.dir != "/выгрузки" || out.tpl.Name != "ЧС Тест" || len(out.tpl.Columns) != 3 || out.tpl.Columns[2].Name != texts.TableComments || len(out.rows) != 1 {
		t.Fatalf("книга: %+v", out)
	}
	r := out.rows[0]
	want := "20.09.2026 10:00 · ЧС Тест · Админ: Тестов Иван — ЧС, не вернул авто\n20.09.2026 10:01 · ЧС Тест · Марат: подтверждаю"
	if r.Cells["ИИН"].Value != "900101300007" || r.Cells[texts.TableComments].Value != want || r.File != "https://t.me/c/55/4" {
		t.Fatalf("строка: %+v", r)
	}
	if err := e.Run(context.Background(), jobsdomain.Job{SourceID: src}, noProgress{}); err != nil || model.calls != 1 {
		t.Fatalf("повтор должен взять строку из кэша: %d %v", model.calls, err)
	}
}

func TestExportWaitsForModel(t *testing.T) {
	e, _, _, src := setup(t)
	e.d.Models = fakeModels{err: analyze.ErrNoModel}
	var wait *jobsservice.WaitUserError
	if err := e.Run(context.Background(), jobsdomain.Job{SourceID: src}, noProgress{}); !errors.As(err, &wait) {
		t.Fatalf("нет модели — ждать: %v", err)
	}
}

func TestSaveRejectsBadColumns(t *testing.T) {
	cfg := NewConfig(nil)
	for cols, want := range map[string][]tables.Column{
		texts.ErrTableNoColumns:     nil,
		"У колонки 2 нет названия":  {{Name: "ИИН", Type: tables.TypeIIN}, {Name: " "}},
		"Колонка «иин» повторяется": {{Name: "ИИН"}, {Name: "иин"}},
		"Название «Комментарии» занято служебной колонкой": {{Name: "Комментарии"}},
	} {
		var p *ProblemError
		if err := cfg.Save(context.Background(), domain.Config{Columns: want}); !errors.As(err, &p) || p.Text != cols {
			t.Fatalf("%q: %v", cols, err)
		}
	}
}
