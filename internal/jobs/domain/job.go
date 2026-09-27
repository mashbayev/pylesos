// Файл: internal/jobs/domain/job.go
// Package domain — задания: всё длительное (сбор, разбор) идёт заданием с курсором продолжения.
package domain

import (
	"database/sql"
	"errors"
)

type Status string

const (
	StatusQueued      Status = "queued"
	StatusRunning     Status = "running"
	StatusPaused      Status = "paused"
	StatusWaitingUser Status = "waiting_user" // ждёт действий пользователя (вход, капча…)
	StatusDone        Status = "done"
	StatusFailed      Status = "failed"
	StatusCancelled   Status = "cancelled"
)

type Action string

const (
	ActionPause  Action = "pause"
	ActionResume Action = "resume"
	ActionCancel Action = "cancel"
)

// Actions — что пользователь может сделать с заданием в этом статусе.
func (s Status) Actions() []Action {
	switch s {
	case StatusRunning:
		return []Action{ActionPause, ActionCancel}
	case StatusPaused, StatusWaitingUser, StatusFailed:
		return []Action{ActionResume, ActionCancel}
	case StatusQueued:
		return []Action{ActionCancel}
	default:
		return []Action{}
	}
}

type Job struct {
	ID           int64         `db:"id"`
	SourceID     int64         `db:"source_id"`
	Kind         string        `db:"kind"`
	Params       string        `db:"params"` // JSON, разбирает исполнитель своего вида
	Status       Status        `db:"status"`
	Cursor       string        `db:"cursor"` // JSON, место продолжения
	ProgressText string        `db:"progress_text"`
	Reason       string        `db:"reason"`
	CreatedAt    int64         `db:"created_at"`
	UpdatedAt    int64         `db:"updated_at"`
	FinishedAt   sql.NullInt64 `db:"finished_at"`
}

// ListItem — задание с названием источника (для экранов).
type ListItem struct {
	Job
	SourceTitle string `db:"source_title"`
}

// StateUpdate — смена статуса. Finished ставит finished_at = At, иначе finished_at очищается.
// From — из каких статусов разрешён переход; пусто — из любого. Защищает от гонок «прочитал — записал».
type StateUpdate struct {
	Status   Status
	Reason   string
	At       int64
	Finished bool
	From     []Status
}

var (
	ErrNotFound   = errors.New("задание не найдено")
	ErrSourceBusy = errors.New("источник занят другим активным заданием")
	// ErrStateChanged — статус задания уже не тот, из которого разрешён переход.
	ErrStateChanged = errors.New("статус задания изменился")
)

// SourceStats — сводка заданий источника для экранов.
type SourceStats struct {
	SourceID int64 `db:"source_id"`
	Active   bool  `db:"active"`    // есть активное задание
	LastDone int64 `db:"last_done"` // когда последнее задание завершилось успешно (0 — никогда)
}
