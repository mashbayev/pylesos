// Файл: internal/telegram/service/sync.go
package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	jobsdomain "pylesos/internal/jobs/domain"
	jobsservice "pylesos/internal/jobs/service"
	sourcesdomain "pylesos/internal/sources/domain"
	"pylesos/internal/telegram/domain"
	"pylesos/internal/telegram/repository"
	"pylesos/internal/texts"
)

type MessageStore interface {
	Save(ctx context.Context, sourceID int64, msgs []domain.Message, via string, at int64) error
	Bounds(ctx context.Context, sourceID int64) (minID, maxID int, err error)
	LiveIDsAfter(ctx context.Context, sourceID int64, afterID, limit int) ([]int, error)
	MarkDeleted(ctx context.Context, sourceID int64, ids []int) error
	SyncState(ctx context.Context, sourceID int64) (repository.SyncState, error)
	SaveSyncState(ctx context.Context, sourceID int64, st repository.SyncState) error
	RecentWindowStart(ctx context.Context, sourceID int64, n int) (int, error)
}

type SourceGetter interface {
	Get(ctx context.Context, id int64) (sourcesdomain.Source, error)
}

// SyncWorker — задание «сбор Telegram»: новые сообщения, история, сверка удалённых (спецификация, раздел 3).
type SyncWorker struct {
	gw       Gateway
	msgs     MessageStore
	sources  SourceGetter
	now      func() time.Time
	sleep    func(context.Context, time.Duration) error
	pageSize int
	window   int // сколько последних сообщений сверять на удаление
}

func NewSyncWorker(gw Gateway, msgs MessageStore, sources SourceGetter, now func() time.Time,
	sleep func(context.Context, time.Duration) error, pageSize, window int) *SyncWorker {
	return &SyncWorker{gw: gw, msgs: msgs, sources: sources, now: now, sleep: sleep, pageSize: pageSize, window: window}
}

func (w *SyncWorker) Kind() string  { return KindSync }
func (w *SyncWorker) Title() string { return texts.TgSyncKind }

const (
	phaseNew     = "new"
	phaseOld     = "old"
	phaseDeleted = "deleted"
	phaseDone    = "done"
)

type syncCursor struct {
	Phase    string `json:"phase"`
	Offset   int    `json:"offset"`   // загружать сообщения с номером меньше этого (0 — с самого нового)
	Top      int    `json:"top"`      // фаза new: всё до этого номера уже загружено (отметка источника)
	Head     int    `json:"head"`     // фаза new: верх прохода (самое новое сообщение на его старте)
	Checked  int    `json:"checked"`  // фаза deleted: сверено до этого номера
	Loaded   int    `json:"loaded"`   // загружено за задание
	Verified int    `json:"verified"` // сверено за задание
}

type syncRun struct {
	w      *SyncWorker
	source int64
	peer   domain.Peer
	p      jobsservice.Progress
	c      syncCursor
}

func (w *SyncWorker) Run(ctx context.Context, job jobsdomain.Job, p jobsservice.Progress) error {
	run, err := w.begin(ctx, job, p)
	if err != nil {
		return err
	}
	for run.c.Phase != phaseDone {
		if err := run.step(ctx); err != nil {
			return stopReason(ctx, err)
		}
	}
	return nil
}

// begin: новое задание продолжает незавершённый проход источника (ревью M2, C1), а не начинает с MAX(msg_id).
func (w *SyncWorker) begin(ctx context.Context, job jobsdomain.Job, p jobsservice.Progress) (*syncRun, error) {
	src, err := w.sources.Get(ctx, job.SourceID)
	if err != nil {
		return nil, err
	}
	var peer domain.Peer
	if err := json.Unmarshal([]byte(src.Config), &peer); err != nil {
		return nil, fmt.Errorf("группа источника %d: %w", job.SourceID, err)
	}
	var c syncCursor
	if err := json.Unmarshal([]byte(job.Cursor), &c); err != nil {
		return nil, fmt.Errorf("курсор задания: %w", err)
	}
	if c.Phase == "" {
		st, err := w.msgs.SyncState(ctx, job.SourceID)
		if err != nil {
			return nil, err
		}
		c = syncCursor{Phase: phaseNew, Top: st.SyncedTop, Head: st.PassHead, Offset: st.PassLow}
	}
	return &syncRun{w: w, source: job.SourceID, peer: peer, p: p, c: c}, nil
}

// stopReason переводит остановку в понятное пользователю: нет входа или доступа — «ждёт вас»,
// прочий сбой — русская причина (спецификация, раздел 7). Пауза и выход проходят как есть.
func stopReason(ctx context.Context, err error) error {
	switch {
	case ctx.Err() != nil:
		return err
	case errors.Is(err, domain.ErrNotAuthorized), errors.Is(err, domain.ErrNotConfigured), errors.Is(err, domain.ErrBadKeys):
		return &jobsservice.WaitUserError{Reason: texts.TgNeedLogin}
	case errors.Is(err, domain.ErrGroupUnavailable):
		return &jobsservice.WaitUserError{Reason: texts.TgGroupUnavailable}
	default:
		return fmt.Errorf("%s: %w", texts.TgSyncFailed, err)
	}
}
