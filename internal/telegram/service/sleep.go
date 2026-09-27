// Файл: internal/telegram/service/sleep.go
package service

import (
	"context"
	"time"
)

// SleepCtx ждёт d или отмены ctx (пауза задания, выход из программы).
func SleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
