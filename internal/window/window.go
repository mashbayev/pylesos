// Файл: internal/window/window.go
package window

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"runtime"
)

// Opener открывает окно программы и файлы. Всё, что зависит от ОС, — поля, чтобы подменять в тестах.
type Opener struct {
	GOOS       string
	LookPath   func(string) (string, error)
	Exists     func(string) bool
	Getenv     func(string) string
	Start      func(name string, args ...string) (wait func() error, err error)
	ProfileDir string // профиль Chromium окна программы
	Log        *slog.Logger
}

// System — Opener для текущей ОС.
func System(profileDir string, log *slog.Logger) Opener {
	return Opener{
		GOOS:     runtime.GOOS,
		LookPath: exec.LookPath,
		Exists: func(p string) bool {
			_, err := os.Stat(p)
			return err == nil
		},
		Getenv:     os.Getenv,
		Start:      startProcess,
		ProfileDir: profileDir,
		Log:        log,
	}
}

func startProcess(name string, args ...string) (func() error, error) {
	cmd := exec.Command(name, args...)
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return cmd.Wait, nil
}

// OpenApp показывает окно программы; канал закрывается, когда пользователь закрыл окно.
func (o Opener) OpenApp(link string) (<-chan struct{}, error) {
	if o.GOOS == "linux" && o.Getenv("DISPLAY") == "" && o.Getenv("WAYLAND_DISPLAY") == "" {
		return nil, ErrNoDisplay
	}
	browser := AppBrowser(o.GOOS, o.LookPath, o.Exists, o.Getenv)
	if browser == "" {
		return nil, ErrNoAppBrowser
	}
	wait, err := o.Start(browser, WindowArgs(link, o.ProfileDir)...)
	if err != nil {
		return nil, fmt.Errorf("запустить окно: %w", err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := wait(); err != nil {
			o.Log.Info("окно программы закрыто", "err", err)
		}
	}()
	return done, nil
}
