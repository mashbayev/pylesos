// Файл: internal/jobs/service/runner.go
package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"time"

	"pylesos/internal/events"
	"pylesos/internal/jobs/domain"
	"pylesos/internal/jobs/repository"
	"pylesos/internal/texts"
)

type runningJob struct {
	cancel context.CancelCauseFunc
	done   chan struct{}
}

type Runner struct {
	base    context.Context // отменяется при выходе из программы
	repo    repository.Repository
	bus     Publisher
	log     *slog.Logger
	now     func() time.Time
	workers map[string]Worker

	mu      sync.Mutex
	running map[int64]runningJob
	wg      sync.WaitGroup
}

func NewRunner(base context.Context, repo repository.Repository, bus Publisher, log *slog.Logger, now func() time.Time, workers ...Worker) *Runner {
	byKind := make(map[string]Worker, len(workers))
	for _, w := range workers {
		byKind[w.Kind()] = w
	}
	return &Runner{base: base, repo: repo, bus: bus, log: log, now: now, workers: byKind, running: map[int64]runningJob{}}
}

// KindTitle — название вида задания для экранов; неизвестный вид показывается как есть.
func (r *Runner) KindTitle(kind string) string {
	if w, ok := r.workers[kind]; ok {
		return w.Title()
	}
	return kind
}

// Start создаёт задание и запускает его.
func (r *Runner) Start(ctx context.Context, sourceID int64, kind, params string) (int64, error) {
	if _, ok := r.workers[kind]; !ok {
		return 0, fmt.Errorf("%w: %s", ErrUnknownKind, kind)
	}
	at := r.now().Unix()
	id, err := r.repo.Create(ctx, domain.Job{SourceID: sourceID, Kind: kind, Params: params,
		Status: domain.StatusQueued, Cursor: "{}", CreatedAt: at, UpdatedAt: at})
	if err != nil {
		return 0, err
	}
	r.changed()
	job, err := r.repo.Get(ctx, id)
	if err != nil {
		return 0, err
	}
	return id, r.launch(ctx, job)
}

// Resume продолжает приостановленное, ждущее пользователя или упавшее задание с сохранённого курсора.
func (r *Runner) Resume(ctx context.Context, id int64) error {
	job, err := r.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	if !slices.Contains(job.Status.Actions(), domain.ActionResume) {
		return ErrNotAllowed
	}
	return r.launch(ctx, job)
}

// Pause останавливает выполняющееся задание и ждёт, пока исполнитель остановится.
func (r *Runner) Pause(ctx context.Context, id int64) error { return r.stop(ctx, id, errPaused) }

// Cancel отменяет задание: выполняющееся — останавливает, остальные (пауза, ждёт, ошибка, очередь) —
// помечает отменёнными. После stop статус в базе уже итоговый: finish пишет его до закрытия done.
func (r *Runner) Cancel(ctx context.Context, id int64) error {
	if err := r.stop(ctx, id, errCancelled); err != nil && !errors.Is(err, ErrNotRunning) {
		return err
	}
	job, err := r.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	if job.Status == domain.StatusCancelled {
		return nil
	}
	if !slices.Contains(job.Status.Actions(), domain.ActionCancel) {
		return ErrNotAllowed
	}
	return r.setState(ctx, id, domain.StatusCancelled, "", true,
		domain.StatusQueued, domain.StatusPaused, domain.StatusWaitingUser, domain.StatusFailed)
}

// ResumeInterrupted при запуске программы продолжает задания, прерванные выходом или сбоем.
func (r *Runner) ResumeInterrupted(ctx context.Context) error {
	jobs, err := r.repo.ListByStatus(ctx, domain.StatusQueued, domain.StatusRunning)
	if err != nil {
		return err
	}
	for _, job := range jobs {
		if err := r.launch(ctx, job); err != nil {
			r.log.Error("продолжить задание", "job", job.ID, "err", err)
		}
	}
	return nil
}

// ResumeWaiting продолжает задания вида kind, ждущие пользователя, — например после входа в Telegram.
func (r *Runner) ResumeWaiting(ctx context.Context, kind string) error {
	jobs, err := r.repo.ListByStatus(ctx, domain.StatusWaitingUser)
	if err != nil {
		return err
	}
	for _, job := range jobs {
		if job.Kind != kind {
			continue
		}
		if err := r.launch(ctx, job); err != nil && !errors.Is(err, ErrNotAllowed) {
			return err
		}
	}
	return nil
}

// Wait ждёт остановки всех исполнителей (после отмены base при выходе из программы).
func (r *Runner) Wait() { r.wg.Wait() }

func (r *Runner) stop(ctx context.Context, id int64, cause error) error {
	r.mu.Lock()
	rj, ok := r.running[id]
	r.mu.Unlock()
	if !ok {
		return ErrNotRunning
	}
	rj.cancel(cause)
	select {
	case <-rj.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// setState меняет статус только из статусов from (гонки «прочитал — записал»);
// статус уже другой → ErrNotAllowed.
func (r *Runner) setState(ctx context.Context, id int64, s domain.Status, reason string, finished bool, from ...domain.Status) error {
	u := domain.StateUpdate{Status: s, Reason: reason, At: r.now().Unix(), Finished: finished, From: from}
	err := r.repo.UpdateState(ctx, id, u)
	if errors.Is(err, domain.ErrStateChanged) {
		return ErrNotAllowed
	}
	if err != nil {
		return err
	}
	r.changed()
	return nil
}

func (r *Runner) changed() { r.bus.Publish(events.Jobs()) }

// unknownKind помечает задание неизвестного вида упавшим.
func (r *Runner) unknownKind(ctx context.Context, job domain.Job) error {
	return errors.Join(fmt.Errorf("%w: %s", ErrUnknownKind, job.Kind),
		r.setState(ctx, job.ID, domain.StatusFailed, texts.JobUnknownKind, true, domain.StatusQueued, domain.StatusRunning))
}
