// Файл: internal/jobs/service/execute.go
package service

import (
	"context"
	"errors"

	"pylesos/internal/jobs/domain"
	"pylesos/internal/texts"
)

// launch регистрирует задание как выполняющееся, переводит в «идёт» и запускает исполнителя.
// Регистрация до смены статуса: Pause/Cancel в этот момент уже видят задание, а два одновременных
// Resume не запустят его дважды.
func (r *Runner) launch(ctx context.Context, job domain.Job) error {
	w, ok := r.workers[job.Kind]
	if !ok {
		return r.unknownKind(ctx, job)
	}
	jobCtx, cancel := context.WithCancelCause(r.base)
	rj := runningJob{cancel: cancel, done: make(chan struct{})}
	if !r.register(job.ID, rj) {
		cancel(nil)
		return ErrNotAllowed
	}
	// running — для ResumeInterrupted после выхода программы.
	if err := r.setState(ctx, job.ID, domain.StatusRunning, "", false, domain.StatusQueued, domain.StatusRunning,
		domain.StatusPaused, domain.StatusWaitingUser, domain.StatusFailed); err != nil {
		r.unregister(job.ID)
		cancel(nil)
		close(rj.done)
		return err
	}
	r.wg.Add(1)
	go r.execute(jobCtx, w, job, rj)
	return nil
}

func (r *Runner) register(id int64, rj runningJob) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, busy := r.running[id]; busy {
		return false
	}
	r.running[id] = rj
	return true
}

func (r *Runner) unregister(id int64) {
	r.mu.Lock()
	delete(r.running, id)
	r.mu.Unlock()
}

// execute: итог записывается до закрытия done, поэтому после stop статус в базе уже итоговый.
func (r *Runner) execute(ctx context.Context, w Worker, job domain.Job, rj runningJob) {
	defer r.wg.Done()
	defer close(rj.done)
	defer r.unregister(job.ID)
	err := r.safeRun(ctx, w, job)
	r.finish(ctx, job.ID, err)
	rj.cancel(nil)
}

// safeRun запускает исполнителя; паника превращается в ошибку задания, а не падение программы.
func (r *Runner) safeRun(ctx context.Context, w Worker, job domain.Job) (err error) {
	defer func() {
		if p := recover(); p != nil {
			r.log.Error("сбой исполнителя", "job", job.ID, "panic", p)
			err = errors.New(texts.JobCrashed)
		}
	}()
	return w.Run(ctx, job, progress{runner: r, id: job.ID})
}

// finish записывает итог. При выходе из программы статус не трогается — задание продолжится при запуске.
func (r *Runner) finish(ctx context.Context, id int64, err error) {
	status, reason, finished, ok := outcome(ctx, r.base, err)
	if !ok {
		return
	}
	if err := r.setState(context.WithoutCancel(ctx), id, status, reason, finished, domain.StatusRunning); err != nil {
		r.log.Error("сохранить итог задания", "job", id, "err", err)
	}
}

// outcome переводит результат исполнителя в статус: (статус, причина, завершено, записывать ли).
func outcome(ctx, base context.Context, err error) (domain.Status, string, bool, bool) {
	var wait *WaitUserError
	cause := context.Cause(ctx)
	switch {
	case err == nil:
		return domain.StatusDone, "", true, true
	case errors.As(err, &wait):
		return domain.StatusWaitingUser, wait.Reason, false, true
	case errors.Is(cause, errPaused):
		return domain.StatusPaused, "", false, true
	case errors.Is(cause, errCancelled):
		return domain.StatusCancelled, "", true, true
	case base.Err() != nil:
		return "", "", false, false
	default:
		return domain.StatusFailed, err.Error(), true, true
	}
}

type progress struct {
	runner *Runner
	id     int64
}

// Save пишет без отмены: порция, сделанная до паузы или выхода, не должна потеряться.
func (p progress) Save(ctx context.Context, cursor, progressText string) error {
	if err := p.runner.repo.SaveCursor(context.WithoutCancel(ctx), p.id, cursor, progressText, p.runner.now().Unix()); err != nil {
		return err
	}
	p.runner.changed()
	return nil
}
