// Файл: internal/jobs/service/runner_test.go
package service

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"pylesos/internal/events"
	"pylesos/internal/jobs/domain"
	"pylesos/internal/jobs/repository"
	"pylesos/internal/store/storetest"
	"pylesos/internal/texts"
)

type fakeWorker struct {
	kind string
	run  func(ctx context.Context, job domain.Job, p Progress) error
}

func (f fakeWorker) Kind() string  { return f.kind }
func (f fakeWorker) Title() string { return "Проверочное задание" }
func (f fakeWorker) Run(ctx context.Context, job domain.Job, p Progress) error {
	return f.run(ctx, job, p)
}

// blocking сохраняет курсор, сообщает о старте и ждёт отмены.
func blocking(started chan<- string) fakeWorker {
	return fakeWorker{kind: "test", run: func(ctx context.Context, job domain.Job, p Progress) error {
		if err := p.Save(ctx, `{"page":1}`, "страница 1"); err != nil {
			return err
		}
		started <- job.Cursor
		<-ctx.Done()
		return ctx.Err()
	}}
}

type env struct {
	repo   *repository.SQLite
	source int64
	log    *slog.Logger
}

func newEnv(t *testing.T) env {
	db := storetest.Open(t)
	return env{repo: repository.NewSQLite(db), source: storetest.AddSource(t, db, "Группа"), log: slog.New(slog.NewJSONHandler(io.Discard, nil))}
}

func (e env) runner(base context.Context, workers ...Worker) *Runner {
	return NewRunner(base, e.repo, events.NewBus(), e.log, time.Now, workers...)
}

func waitStatus(t *testing.T, e env, id int64, want domain.Status) domain.Job {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second) // с запасом на загруженную машину
	for {
		job, err := e.repo.Get(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		if job.Status == want {
			return job
		}
		if time.Now().After(deadline) {
			t.Fatalf("статус %q, ждали %q (причина %q)", job.Status, want, job.Reason)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestStartRunsToDoneAndWaitUser(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	done := fakeWorker{kind: "done", run: func(context.Context, domain.Job, Progress) error { return nil }}
	wait := fakeWorker{kind: "wait", run: func(context.Context, domain.Job, Progress) error {
		return &WaitUserError{Reason: "Войдите в OLX"}
	}}
	r := e.runner(ctx, done, wait)
	id, err := r.Start(ctx, e.source, "done", "{}")
	if err != nil {
		t.Fatal(err)
	}
	if job := waitStatus(t, e, id, domain.StatusDone); !job.FinishedAt.Valid {
		t.Fatal("finished_at не заполнен")
	}
	id, err = r.Start(ctx, e.source, "wait", "{}")
	if err != nil {
		t.Fatal(err)
	}
	if job := waitStatus(t, e, id, domain.StatusWaitingUser); job.Reason != "Войдите в OLX" {
		t.Fatalf("причина %q", job.Reason)
	}
}

func TestPauseAndResumeFromCursor(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	started := make(chan string, 2)
	r := e.runner(ctx, blocking(started))
	id, err := r.Start(ctx, e.source, "test", "{}")
	if err != nil {
		t.Fatal(err)
	}
	<-started
	if err := r.Pause(ctx, id); err != nil {
		t.Fatal(err)
	}
	if job := waitStatus(t, e, id, domain.StatusPaused); job.ProgressText != "страница 1" {
		t.Fatalf("прогресс %q", job.ProgressText)
	}
	if err := r.Resume(ctx, id); err != nil {
		t.Fatal(err)
	}
	if cursor := <-started; cursor != `{"page":1}` {
		t.Fatalf("продолжение не с курсора: %q", cursor)
	}
	if err := r.Cancel(ctx, id); err != nil {
		t.Fatal(err)
	}
	waitStatus(t, e, id, domain.StatusCancelled)
	if err := r.Pause(ctx, id); !errors.Is(err, ErrNotRunning) {
		t.Fatalf("пауза невыполняющегося: %v", err)
	}
}

func TestShutdownKeepsJobAndResumes(t *testing.T) {
	e := newEnv(t)
	base, shutdown := context.WithCancel(context.Background())
	started := make(chan string, 1)
	r := e.runner(base, blocking(started))
	id, err := r.Start(base, e.source, "test", "{}")
	if err != nil {
		t.Fatal(err)
	}
	<-started
	shutdown()
	r.Wait()
	waitStatus(t, e, id, domain.StatusRunning) // выход из программы — не пауза и не ошибка
	var gotCursor string
	resumed := fakeWorker{kind: "test", run: func(_ context.Context, job domain.Job, _ Progress) error {
		gotCursor = job.Cursor
		return nil
	}}
	ctx := context.Background()
	r2 := e.runner(ctx, resumed)
	if err := r2.ResumeInterrupted(ctx); err != nil {
		t.Fatal(err)
	}
	waitStatus(t, e, id, domain.StatusDone)
	r2.Wait()
	if gotCursor != `{"page":1}` {
		t.Fatalf("курсор после перезапуска: %q", gotCursor)
	}
}

func TestBusySourceFailureAndPanic(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	started := make(chan string, 1)
	fail := fakeWorker{kind: "fail", run: func(context.Context, domain.Job, Progress) error { return errors.New("сеть недоступна") }}
	boom := fakeWorker{kind: "boom", run: func(context.Context, domain.Job, Progress) error { panic("сломалось") }}
	r := e.runner(ctx, blocking(started), fail, boom)
	id, err := r.Start(ctx, e.source, "test", "{}")
	if err != nil {
		t.Fatal(err)
	}
	<-started
	if _, err := r.Start(ctx, e.source, "fail", "{}"); !errors.Is(err, domain.ErrSourceBusy) {
		t.Fatalf("второе задание на источник: %v", err)
	}
	if err := r.Cancel(ctx, id); err != nil {
		t.Fatal(err)
	}
	id, err = r.Start(ctx, e.source, "fail", "{}")
	if err != nil {
		t.Fatal(err)
	}
	if job := waitStatus(t, e, id, domain.StatusFailed); job.Reason != "сеть недоступна" {
		t.Fatalf("причина %q", job.Reason)
	}
	if err := r.Cancel(ctx, id); err != nil {
		t.Fatal(err)
	}
	id, err = r.Start(ctx, e.source, "boom", "{}")
	if err != nil {
		t.Fatal(err)
	}
	if job := waitStatus(t, e, id, domain.StatusFailed); job.Reason != texts.JobCrashed {
		t.Fatalf("причина %q", job.Reason)
	}
}

func TestUnknownKind(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	r := e.runner(ctx)
	if _, err := r.Start(ctx, e.source, "gone", "{}"); !errors.Is(err, ErrUnknownKind) {
		t.Fatalf("Start: %v", err)
	}
	id, err := e.repo.Create(ctx, domain.Job{SourceID: e.source, Kind: "gone", Params: "{}", Status: domain.StatusRunning, Cursor: "{}"})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.ResumeInterrupted(ctx); err != nil {
		t.Fatal(err)
	}
	if job := waitStatus(t, e, id, domain.StatusFailed); job.Reason != texts.JobUnknownKind {
		t.Fatalf("причина %q", job.Reason)
	}
	if r.KindTitle("gone") != "gone" {
		t.Fatal("неизвестный вид должен показываться как есть")
	}
}

// Гонка: Resume прочитал «пауза», Cancel успел отменить — запуск не должен перезаписать «отменено».
func TestStaleResumeDoesNotReviveCancelled(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	ran := make(chan struct{}, 1)
	w := fakeWorker{kind: "test", run: func(context.Context, domain.Job, Progress) error { ran <- struct{}{}; return nil }}
	r := e.runner(ctx, w)
	id, err := e.repo.Create(ctx, domain.Job{SourceID: e.source, Kind: "test", Params: "{}", Status: domain.StatusPaused, Cursor: "{}"})
	if err != nil {
		t.Fatal(err)
	}
	stale, err := e.repo.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Cancel(ctx, id); err != nil {
		t.Fatal(err)
	}
	if err := r.launch(ctx, stale); !errors.Is(err, ErrNotAllowed) {
		t.Fatalf("запуск по устаревшему статусу: %v", err)
	}
	r.Wait()
	select {
	case <-ran:
		t.Fatal("исполнитель отменённого задания запустился")
	default:
	}
	waitStatus(t, e, id, domain.StatusCancelled)
	if err := r.launch(ctx, stale); !errors.Is(err, ErrNotAllowed) {
		t.Fatalf("повторный запуск: %v", err) // регистрация снята после отказа
	}
}

// Курсор, сохранённый уже после команды «Пауза», не теряется (П11: не тратить открытые телефоны заново).
func TestSaveAfterPauseKeepsCursor(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	started := make(chan struct{}, 1)
	w := fakeWorker{kind: "test", run: func(ctx context.Context, _ domain.Job, p Progress) error {
		started <- struct{}{}
		<-ctx.Done()
		if err := p.Save(ctx, `{"page":2}`, "страница 2"); err != nil {
			return err
		}
		return ctx.Err()
	}}
	r := e.runner(ctx, w)
	id, err := r.Start(ctx, e.source, "test", "{}")
	if err != nil {
		t.Fatal(err)
	}
	<-started
	if err := r.Pause(ctx, id); err != nil {
		t.Fatal(err)
	}
	job := waitStatus(t, e, id, domain.StatusPaused)
	if job.Cursor != `{"page":2}` {
		t.Fatalf("курсор %q, причина %q", job.Cursor, job.Reason)
	}
}

// Ревью M2, I5: после входа задания нужного вида, ждущие пользователя, продолжаются сами.
func TestResumeWaitingOnlyOfKind(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	done := fakeWorker{kind: "tg", run: func(context.Context, domain.Job, Progress) error { return nil }}
	other := fakeWorker{kind: "olx", run: func(context.Context, domain.Job, Progress) error { return nil }}
	r := e.runner(ctx, done, other)
	tg, err := e.repo.Create(ctx, domain.Job{SourceID: e.source, Kind: "tg", Params: "{}", Status: domain.StatusWaitingUser, Cursor: "{}"})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.ResumeWaiting(ctx, "tg"); err != nil {
		t.Fatal(err)
	}
	waitStatus(t, e, tg, domain.StatusDone)
	if err := r.ResumeWaiting(ctx, "tg"); err != nil {
		t.Fatalf("нечего продолжать — не ошибка: %v", err)
	}
}
