// Файл: internal/analyze/worker_test.go
package analyze

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	docrepo "pylesos/internal/docforms/repository"
	jobsdomain "pylesos/internal/jobs/domain"
	jobsservice "pylesos/internal/jobs/service"
	"pylesos/internal/normalize"
	pdomain "pylesos/internal/people/domain"
	prepo "pylesos/internal/people/repository"
	sourcesdomain "pylesos/internal/sources/domain"
	sourcesrepo "pylesos/internal/sources/repository"
	"pylesos/internal/store/storetest"
	tgdomain "pylesos/internal/telegram/domain"
	tgrepo "pylesos/internal/telegram/repository"
	"pylesos/internal/texts"
)

type fakeGateway struct{ files map[int][]tgdomain.File }

func (f fakeGateway) Media(_ context.Context, _ tgdomain.Peer, msgID int) ([]tgdomain.File, error) {
	return f.files[msgID], nil
}

type fakePicker struct{ err error }

func (f fakePicker) Pick(context.Context) (string, error) { return "m", f.err }

type recProgress struct{ texts []string }

func (p *recProgress) Save(_ context.Context, _ string, text string) error {
	p.texts = append(p.texts, text)
	return nil
}

type workerEnv struct {
	db     *sqlx.DB
	w      *Worker
	msgs   *tgrepo.SQLite
	people *prepo.SQLite
	source int64
	llm    *fakeLLM
}

func newWorkerEnv(t *testing.T, picker ModelPicker) *workerEnv {
	t.Helper()
	db := storetest.Open(t)
	sources := sourcesrepo.NewSQLite(db)
	cfg, err := json.Marshal(tgdomain.Peer{Type: tgdomain.PeerChannel, ID: 1, AccessHash: 2})
	if err != nil {
		t.Fatal(err)
	}
	src, _, err := sources.Ensure(context.Background(), sourcesdomain.Source{Kind: sourcesdomain.KindTelegramGroup, Title: "Группа", Config: string(cfg)})
	if err != nil {
		t.Fatal(err)
	}
	e := &workerEnv{db: db, msgs: tgrepo.NewSQLite(db), people: prepo.NewSQLite(db), source: src, llm: &fakeLLM{}}
	e.w = NewWorker(Deps{Gateway: fakeGateway{files: map[int][]tgdomain.File{2: {{Name: "photo.png", Data: docPNG(t)}}}},
		Messages: e.msgs, People: e.people, Sources: sources, Pages: normalize.New(), Forms: docrepo.NewSQLite(db),
		LLM: e.llm, Models: picker, Now: func() time.Time { return time.Unix(100, 0) }})
	return e
}

// Вымышленная переписка в духе «ЧС Астана» (П8).
func (e *workerEnv) seed(t *testing.T) {
	t.Helper()
	photo := tgdomain.Message{ID: 2, Text: "ЧС. Бросил авто, аренду не платит", Media: []tgdomain.Media{{Kind: tgdomain.MediaPhoto}}}
	msgs := []tgdomain.Message{
		{ID: 1, Text: "Тестов Иван ЧС! Долг по аренде"},
		photo,
		{ID: 3, Text: "Подтверждаю, должен", ReplyTo: 1},
		{ID: 4, Text: "Примеров Пётр. Есть инфо?"},
		{ID: 5, Text: ""},
	}
	if err := e.msgs.Save(context.Background(), e.source, msgs, tgrepo.ViaAccount, 1); err != nil {
		t.Fatal(err)
	}
	e.llm.docText = "Удостоверение личности. ТЕСТОВ ИВАН. ИИН 900101300007"
	e.llm.docRow = `{"rows":[{"ФИО":{"value":"ТЕСТОВ ИВАН","confidence":"high"},"ИИН":{"value":"900101300007","confidence":"high"},` +
		`"Номер документа":{"value":"012345678","confidence":"high"},"Дата рождения":{"value":"01.01.1990","confidence":"high"}}]}`
	e.llm.mentions = map[string]string{
		"Тестов Иван ЧС!": mentionsJSON(t, Found{Name: "Тестов Иван", Role: "record"}),
		"Примеров Пётр":   mentionsJSON(t, Found{Name: "Примеров Пётр", Role: "query"}),
	}
}

func (e *workerEnv) roles(t *testing.T, msgID int) []pdomain.Mention {
	t.Helper()
	var list []pdomain.Mention
	if err := e.db.Select(&list, `SELECT id, source_id, msg_id, person_id, role, name_as_written, link FROM mentions
		WHERE source_id = ? AND msg_id = ? ORDER BY id`, e.source, msgID); err != nil {
		t.Fatal(err)
	}
	return list
}

func TestAnalyzeGroup(t *testing.T) {
	e := newWorkerEnv(t, fakePicker{})
	e.seed(t)
	p := &recProgress{}
	if err := e.w.Run(context.Background(), jobsdomain.Job{SourceID: e.source, Cursor: "{}"}, p); err != nil {
		t.Fatal(err)
	}
	record := e.roles(t, 1)
	if len(record) != 1 || record[0].Role != pdomain.RoleRecord || !record[0].PersonID.Valid {
		t.Fatalf("запись: %+v", record)
	}
	doc := e.roles(t, 2)
	if len(doc) != 1 || doc[0].Link != pdomain.LinkDocument {
		t.Fatalf("фото без имени в подписи — человек из документа: %+v", doc)
	}
	reply := e.roles(t, 3)
	if len(reply) != 1 || reply[0].Role != pdomain.RoleComment || reply[0].PersonID != record[0].PersonID {
		t.Fatalf("ответ — комментарий к тому же человеку: %+v", reply)
	}
	if q := e.roles(t, 4); len(q) != 1 || q[0].Role != pdomain.RoleQuery {
		t.Fatalf("запрос: %+v", q)
	}
	if len(e.roles(t, 5)) != 0 {
		t.Fatal("у пустого сообщения упоминаний быть не должно")
	}
	cands, err := e.people.Candidates(context.Background())
	if err != nil || len(cands) != 1 {
		t.Fatalf("«Тестов Иван» без ИИН и с ИИН из документа — пара «возможно, один человек»: %+v %v", cands, err)
	}
	if pending, err := e.msgs.PendingAnalysis(context.Background(), e.source, 10); err != nil || len(pending) != 0 {
		t.Fatalf("не всё разобрано: %+v %v", pending, err)
	}
	if last := p.texts[len(p.texts)-1]; last != texts.AnalyzeDone {
		t.Fatalf("прогресс: %v", p.texts)
	}
}

func TestAnalyzeWithoutModelWaitsForUser(t *testing.T) {
	e := newWorkerEnv(t, fakePicker{err: ErrNoModel})
	err := e.w.Run(context.Background(), jobsdomain.Job{SourceID: e.source, Cursor: "{}"}, &recProgress{})
	var wait *jobsservice.WaitUserError
	if !errors.As(err, &wait) || wait.Reason != texts.AnalyzeNeedModel {
		t.Fatalf("без модели: %v", err)
	}
}

// Ответ называет человека по имени, а тёзок двое: берём того, о ком исходное сообщение.
func TestReplyNamesAmbiguousPerson(t *testing.T) {
	e := newWorkerEnv(t, fakePicker{})
	e.seed(t)
	e.llm.mentions["Подтверждаю"] = mentionsJSON(t, Found{Name: "Тестов Иван", Role: "comment"})
	if err := e.w.Run(context.Background(), jobsdomain.Job{SourceID: e.source, Cursor: "{}"}, &recProgress{}); err != nil {
		t.Fatal(err)
	}
	record, reply := e.roles(t, 1), e.roles(t, 3)
	if len(reply) != 1 || reply[0].PersonID != record[0].PersonID || reply[0].Link != pdomain.LinkReply {
		t.Fatalf("тёзка из исходного сообщения: %+v (запись %+v)", reply, record)
	}
}
