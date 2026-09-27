// Файл: internal/jobs/service/worker.go
// Package service — выполнение заданий: статусы, пауза, продолжение, продолжение после перезапуска.
package service

import (
	"context"
	"errors"

	"pylesos/internal/events"
	"pylesos/internal/jobs/domain"
)

// Worker — исполнитель заданий одного вида (история Telegram, поиск OLX, разбор… — с M2).
type Worker interface {
	Kind() string  // вид задания, хранится в jobs.kind
	Title() string // название вида для экранов, например «История Telegram»
	// Run выполняет задание с места job.Cursor и сохраняет курсор через p.Save после каждой порции.
	// Возвращает nil — готово; *WaitUserError — нужны действия пользователя; ctx.Err() — пауза, отмена
	// или выход; другую ошибку — сбой (её текст увидит пользователь, поэтому по-русски).
	Run(ctx context.Context, job domain.Job, p Progress) error
}

// Progress — сохранение курсора и строки прогресса выполняющегося задания.
type Progress interface {
	Save(ctx context.Context, cursor, progressText string) error
}

// WaitUserError — задание ждёт пользователя (войти, пройти капчу…). Reason — готовый текст для экрана.
type WaitUserError struct{ Reason string }

func (e *WaitUserError) Error() string { return e.Reason }

// Publisher — куда сообщать об изменениях заданий.
type Publisher interface{ Publish(events.Event) }

var (
	ErrUnknownKind = errors.New("неизвестный вид задания")
	ErrNotRunning  = errors.New("задание сейчас не выполняется")
	ErrNotAllowed  = errors.New("действие недоступно в этом статусе")
	errPaused      = errors.New("пауза")
	errCancelled   = errors.New("отмена")
)
