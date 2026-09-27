// Файл: internal/telegram/service/sync_retry.go
package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"pylesos/internal/telegram/domain"
	"pylesos/internal/texts"
)

// Паузы при отсутствии связи: 30 с, дальше вдвое, не больше 5 минут (спецификация, раздел 7).
const (
	retryFirst = 30 * time.Second
	retryMax   = 5 * time.Minute
)

// history — порция ниже Offset и выше minID с повторами после FLOOD_WAIT и обрыва связи.
func (r *syncRun) history(ctx context.Context, minID int) (domain.Page, error) {
	return retry(ctx, r, func() (domain.Page, error) {
		return r.w.gw.History(ctx, r.peer, r.c.Offset, minID, r.w.pageSize)
	})
}

// retry повторяет вызов после FLOOD_WAIT (ждём, сколько просит Telegram) и «нет связи»
// (нарастающая пауза), показывая ожидание в прогрессе.
func retry[T any](ctx context.Context, r *syncRun, call func() (T, error)) (T, error) {
	backoff := retryFirst
	for {
		res, err := call()
		wait, text := waitFor(err, backoff)
		if wait == 0 {
			return res, err
		}
		var zero T
		if err := r.save(ctx, text); err != nil {
			return zero, err
		}
		if err := r.w.sleep(ctx, wait); err != nil {
			return zero, err
		}
		if errors.Is(err, domain.ErrUnavailable) {
			backoff = min(backoff*2, retryMax)
		}
	}
}

// waitFor — сколько ждать перед повтором и что показать; 0 — не повторять.
func waitFor(err error, backoff time.Duration) (time.Duration, string) {
	var flood *domain.FloodWaitError
	switch {
	case errors.As(err, &flood):
		return flood.Wait, fmt.Sprintf(texts.TgFloodWait, int(flood.Wait.Seconds()))
	case errors.Is(err, domain.ErrUnavailable):
		return backoff, fmt.Sprintf(texts.TgNoConnectionRetry, int(backoff.Seconds()))
	default:
		return 0, ""
	}
}
