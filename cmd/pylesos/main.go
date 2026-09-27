// Файл: cmd/pylesos/main.go
// Пылесос: сбор данных из Telegram-групп и сайтов объявлений в Excel (docs/specs/2026-09-26-pylesos-design.md).
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"pylesos/internal/instance"
	"pylesos/internal/logger"
	"pylesos/internal/paths"
	"pylesos/internal/store"
)

func main() {
	log := logger.New(os.Stderr, slog.LevelInfo)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := run(ctx, log)
	stop()
	if err != nil {
		log.Error("программа остановлена с ошибкой", "err", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, log *slog.Logger) error {
	p, err := resolvePaths()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(p.DataDir, 0o700); err != nil {
		return fmt.Errorf("папка данных: %w", err)
	}
	instanceFile := filepath.Join(p.DataDir, "instance.json")
	release, proceed, err := acquireInstance(ctx, p.DataDir, instanceFile, log)
	if err != nil || !proceed {
		return err
	}
	defer closeLogged(log, "блокировку экземпляра", release)
	db, err := store.Open(ctx, filepath.Join(p.DataDir, "pylesos.db"))
	if err != nil {
		return err
	}
	defer closeLogged(log, "базу", db.Close)
	appCtx, cancelApp := context.WithCancel(ctx)
	defer cancelApp()
	a, err := wire(appCtx, db, p, log)
	if err != nil {
		return err
	}
	return serve(ctx, cancelApp, a, p, instanceFile, log)
}

// acquireInstance берёт блокировку папки данных до открытия базы. Не взяли — работает другая копия:
// просим её показать окно (она может ещё стартовать) и выходим.
func acquireInstance(ctx context.Context, dataDir, instanceFile string, log *slog.Logger) (func() error, bool, error) {
	release, ok, err := instance.Lock(filepath.Join(dataDir, "instance.lock"))
	if err != nil {
		return nil, false, err
	}
	if ok {
		return release, true, nil
	}
	if instance.ShowRunningWithin(ctx, http.DefaultClient, instanceFile, 15*time.Second, 300*time.Millisecond) {
		log.Info("программа уже запущена — показываю её окно")
	} else {
		log.Warn("другая копия программы держит папку данных, но не отвечает")
	}
	return nil, false, nil
}

func resolvePaths() (paths.Paths, error) {
	cfg, err := os.UserConfigDir()
	if err != nil {
		return paths.Paths{}, fmt.Errorf("папка настроек ОС: %w", err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return paths.Paths{}, fmt.Errorf("домашняя папка: %w", err)
	}
	return paths.Resolve(cfg, paths.DocumentsDir(runtime.GOOS, home, os.ReadFile)), nil
}

func closeLogged(log *slog.Logger, what string, closeFn func() error) {
	if err := closeFn(); err != nil {
		log.Error("не удалось закрыть "+what, "err", err)
	}
}
