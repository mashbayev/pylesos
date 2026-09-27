// Файл: internal/telegram/service/sync_test.go
package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	jobsdomain "pylesos/internal/jobs/domain"
	jobsservice "pylesos/internal/jobs/service"
	sourcesdomain "pylesos/internal/sources/domain"
	sourcesrepo "pylesos/internal/sources/repository"
	"pylesos/internal/store/storetest"
	"pylesos/internal/telegram/domain"
	tgrepo "pylesos/internal/telegram/repository"
	"pylesos/internal/texts"
)

type recProgress struct {
	mu     sync.Mutex
	cursor string
	texts  []string
}

func (p *recProgress) Save(_ context.Context, cursor, text string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cursor = cursor
	p.texts = append(p.texts, text)
	return nil
}

type syncEnv struct {
	gw     *fakeGateway
	w      *SyncWorker
	msgs   *tgrepo.SQLite
	source int64
	slept  []time.Duration
}

func newSyncEnv(t *testing.T, ids ...int) *syncEnv {
	t.Helper()
	db := storetest.Open(t)
	sources := sourcesrepo.NewSQLite(db)
	cfg, err := json.Marshal(domain.Peer{Type: domain.PeerChannel, ID: 10, AccessHash: 20})
	if err != nil {
		t.Fatal(err)
	}
	id, _, err := sources.Ensure(context.Background(), sourcesdomain.Source{Kind: sourcesdomain.KindTelegramGroup,
		Title: "Тестовая группа", Config: string(cfg)})
	if err != nil {
		t.Fatal(err)
	}
	e := &syncEnv{gw: &fakeGateway{authorized: true, msgs: map[int]domain.Message{}}, msgs: tgrepo.NewSQLite(db), source: id}
	for _, n := range ids {
		e.gw.msgs[n] = domain.Message{ID: n, Date: int64(n), Text: fmt.Sprintf("сообщение %d", n)}
	}
	sleep := func(_ context.Context, d time.Duration) error { e.slept = append(e.slept, d); return nil }
	e.w = NewSyncWorker(e.gw, e.msgs, sources, func() time.Time { return time.Unix(5000, 0) }, sleep, 2, 1000)
	return e
}

func (e *syncEnv) run(t *testing.T, cursor string) (*recProgress, error) {
	t.Helper()
	p := &recProgress{}
	err := e.w.Run(context.Background(), jobsdomain.Job{SourceID: e.source, Cursor: cursor}, p)
	return p, err
}

func (e *syncEnv) count(t *testing.T) int {
	t.Helper()
	counts, err := e.msgs.Counts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return counts[e.source]
}

func TestFirstSyncLoadsWholeHistoryInPages(t *testing.T) {
	e := newSyncEnv(t, 1, 2, 3, 4, 5)
	p, err := e.run(t, "{}")
	if err != nil {
		t.Fatal(err)
	}
	if n := e.count(t); n != 5 {
		t.Fatalf("сообщений %d", n)
	}
	if !strings.Contains(p.cursor, `"phase":"done"`) || p.texts[len(p.texts)-1] != fmt.Sprintf(texts.TgProgressDone, 5) {
		t.Fatalf("курсор %s, прогресс %v", p.cursor, p.texts)
	}
}

func TestNextSyncLoadsOnlyNewAndMarksDeleted(t *testing.T) {
	e := newSyncEnv(t, 1, 2, 3, 4, 5)
	if _, err := e.run(t, "{}"); err != nil {
		t.Fatal(err)
	}
	e.gw.msgs[6] = domain.Message{ID: 6, Text: "новое"}
	e.gw.msgs[7] = domain.Message{ID: 7, Text: "ещё новое"}
	delete(e.gw.msgs, 2)
	e.gw.minIDs = nil
	if _, err := e.run(t, "{}"); err != nil {
		t.Fatal(err)
	}
	if n := e.count(t); n != 6 {
		t.Fatalf("сообщений %d (7 минус удалённое)", n)
	}
	if e.gw.minIDs[0] != 5 {
		t.Fatalf("новые запрошены не от последнего сохранённого: minID %v", e.gw.minIDs)
	}
	ids, err := e.msgs.LiveIDsAfter(context.Background(), e.source, 0, 10)
	if err != nil || len(ids) != 6 || ids[1] != 3 {
		t.Fatalf("живые %v %v", ids, err)
	}
}

func TestResumeFromCursor(t *testing.T) {
	e := newSyncEnv(t, 1, 2, 3, 4, 5)
	if _, err := e.run(t, `{"phase":"old","offset":4}`); err != nil {
		t.Fatal(err)
	}
	if n := e.count(t); n != 3 {
		t.Fatalf("после продолжения с №4 загружено %d, ждали 3 (1–3)", n)
	}
}

func TestFloodWaitWaitsAndContinues(t *testing.T) {
	e := newSyncEnv(t, 1, 2, 3, 4, 5)
	e.gw.flood = 7 * time.Second
	p, err := e.run(t, "{}")
	if err != nil {
		t.Fatal(err)
	}
	if len(e.slept) != 1 || e.slept[0] != 7*time.Second {
		t.Fatalf("ожидания %v", e.slept)
	}
	if p.texts[0] != fmt.Sprintf(texts.TgFloodWait, 7) || e.count(t) != 5 {
		t.Fatalf("прогресс %v, сообщений %d", p.texts, e.count(t))
	}
}

func TestNotAuthorizedWaitsForUser(t *testing.T) {
	e := newSyncEnv(t, 1)
	e.gw.historyErr = fmt.Errorf("сессия: %w", domain.ErrNotAuthorized)
	_, err := e.run(t, "{}")
	var wait *jobsservice.WaitUserError
	if !errors.As(err, &wait) || wait.Reason != texts.TgNeedLogin {
		t.Fatalf("ждали «войдите в Telegram», получили %v", err)
	}
}

// Ревью M2, C1: задание оборвалось посреди прохода «новые» — следующее задание не оставляет дыру.
func TestInterruptedNewPassLeavesNoGap(t *testing.T) {
	e := newSyncEnv(t, 1, 2, 3, 4, 5)
	if _, err := e.run(t, "{}"); err != nil {
		t.Fatal(err)
	}
	for n := 6; n <= 11; n++ {
		e.gw.msgs[n] = domain.Message{ID: n, Text: "новое"}
	}
	e.gw.calls, e.gw.failAt = 0, 2 // первая порция (11, 10) сохранена, на второй — обрыв
	if _, err := e.run(t, "{}"); err == nil {
		t.Fatal("ждали обрыв")
	}
	e.gw.failAt = 0
	if _, err := e.run(t, "{}"); err != nil {
		t.Fatal(err)
	}
	ids, err := e.msgs.LiveIDsAfter(context.Background(), e.source, 0, 100)
	if err != nil || len(ids) != 11 {
		t.Fatalf("после обрыва и нового задания сообщения %v (ждали 1–11 без пропусков) %v", ids, err)
	}
}

// Ревью M2, I4: порция из одних служебных сообщений не означает конец истории.
func TestServicePageDoesNotEndHistory(t *testing.T) {
	e := newSyncEnv(t, 1, 2, 3)
	e.gw.service = map[int]bool{4: true, 5: true, 6: true, 7: true}
	if _, err := e.run(t, "{}"); err != nil {
		t.Fatal(err)
	}
	if n := e.count(t); n != 3 {
		t.Fatalf("под служебными сообщениями загружено %d из 3", n)
	}
}

// Ревью M2, I6: удалённые сверяются только в последних сообщениях, а не по всей истории.
func TestDeletedCheckOnlyRecentWindow(t *testing.T) {
	e := newSyncEnv(t, 1, 2, 3, 4, 5)
	if _, err := e.run(t, "{}"); err != nil {
		t.Fatal(err)
	}
	e.w.window = 3
	delete(e.gw.msgs, 1)
	delete(e.gw.msgs, 5)
	if _, err := e.run(t, "{}"); err != nil {
		t.Fatal(err)
	}
	ids, err := e.msgs.LiveIDsAfter(context.Background(), e.source, 0, 100)
	if err != nil || len(ids) != 4 || ids[0] != 1 {
		t.Fatalf("живые %v: №5 (в окне) должно быть удалено, №1 (вне окна) — нет; %v", ids, err)
	}
}

// Ревью M2, I3: нет связи — сбор ждёт и повторяет с нарастающей паузой (спецификация, раздел 7).
func TestNoConnectionRetries(t *testing.T) {
	e := newSyncEnv(t, 1, 2, 3)
	e.gw.unavailable = 2
	p, err := e.run(t, "{}")
	if err != nil {
		t.Fatal(err)
	}
	if len(e.slept) != 2 || e.slept[0] != 30*time.Second || e.slept[1] != 60*time.Second {
		t.Fatalf("паузы %v", e.slept)
	}
	if p.texts[0] != fmt.Sprintf(texts.TgNoConnectionRetry, 30) || e.count(t) != 3 {
		t.Fatalf("прогресс %v, сообщений %d", p.texts, e.count(t))
	}
}

// Ревью M2, I7: причины остановки — готовые русские фразы.
func TestGroupUnavailableAndUnexpectedErrors(t *testing.T) {
	e := newSyncEnv(t, 1)
	e.gw.historyErr = fmt.Errorf("CHANNEL_PRIVATE: %w", domain.ErrGroupUnavailable)
	_, err := e.run(t, "{}")
	var wait *jobsservice.WaitUserError
	if !errors.As(err, &wait) || wait.Reason != texts.TgGroupUnavailable {
		t.Fatalf("нет доступа к группе: %v", err)
	}
	e.gw.historyErr = errors.New("internal server error")
	if _, err := e.run(t, "{}"); err == nil || !strings.HasPrefix(err.Error(), texts.TgSyncFailed) {
		t.Fatalf("непредвиденная ошибка без русской причины: %v", err)
	}
}
