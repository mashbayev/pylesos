// Файл: cmd/pylesos/serve.go
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"path/filepath"
	"time"

	"pylesos/internal/instance"
	"pylesos/internal/lifecycle"
	"pylesos/internal/paths"
	"pylesos/internal/window"
)

// serve запускает HTTP-сервер и окно, ждёт выхода и останавливает всё по порядку:
// потоки событий → HTTP (новых действий больше нет) → задания (курсоры уже сохранены).
func serve(ctx context.Context, cancelApp context.CancelFunc, a app, p paths.Paths, instanceFile string, log *slog.Logger) error {
	ln, err := a.server.Listen()
	if err != nil {
		return err
	}
	httpSrv := &http.Server{Handler: a.server.Handler(), ReadHeaderTimeout: 10 * time.Second}
	serveErr := make(chan error, 1)
	go func() { serveErr <- httpSrv.Serve(ln) }()
	if err := instance.Write(instanceFile, a.server.URL()); err != nil {
		log.Warn("файл экземпляра не записан — повторный запуск не покажет окно", "err", err)
	}
	opener := window.System(filepath.Join(p.DataDir, "window"), log)
	sig := lifecycle.Signals{Quit: a.server.Quit(), Show: a.server.Show(), ServeErr: serveErr}
	waitErr := lifecycle.WaitForExit(ctx, a.server.URL(), opener, sig, time.Now, log)
	a.server.StopStreams()
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	shutdownErr := httpSrv.Shutdown(shutdownCtx)
	cancelApp()
	a.runner.Wait()
	<-a.bot.Done() // цикл бота остановлен до закрытия базы
	return errors.Join(waitErr, shutdownErr)
}
