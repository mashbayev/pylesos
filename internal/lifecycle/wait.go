// Файл: internal/lifecycle/wait.go
// Package lifecycle — окно программы и выход: когда открыть окно, когда перейти в браузер, когда завершиться.
package lifecycle

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"pylesos/internal/window"
)

// Window — окно программы (window.Opener).
type Window interface {
	OpenApp(link string) (<-chan struct{}, error)
	OpenInOS(target string, dir bool) error
}

// Signals — поводы выйти или показать окно.
type Signals struct {
	Quit     <-chan struct{} // кнопка «Выход»
	Show     <-chan struct{} // повторный запуск программы
	ServeErr <-chan error    // сбой HTTP-сервера
}

// state — отслеживаемое окно; done == nil — окна нет, работаем во вкладке браузера (выход — кнопкой).
type state struct {
	done   <-chan struct{}
	opened time.Time
}

// WaitForExit показывает окно и ждёт выхода: закрытие окна, «Выход», отмена ctx или сбой сервера.
func WaitForExit(ctx context.Context, link string, w Window, sig Signals, now func() time.Time, log *slog.Logger) error {
	cur := open(link, w, now, log)
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-sig.Quit:
			return nil
		case err := <-sig.ServeErr:
			return fmt.Errorf("HTTP-сервер: %w", err)
		case <-sig.Show:
			cur = show(link, w, cur, now, log)
		case <-cur.done:
			if window.LivedLongEnough(now().Sub(cur.opened)) {
				return nil
			}
			log.Warn("окно программы закрылось сразу — открываю в браузере по умолчанию")
			browser(link, w, log)
			cur = state{}
		}
	}
}

// open открывает окно; не открылось — страница в браузере по умолчанию.
func open(link string, w Window, now func() time.Time, log *slog.Logger) state {
	opened := now()
	done, err := w.OpenApp(link)
	if err != nil {
		log.Warn("окно программы не открылось — открываю в браузере по умолчанию", "err", err)
		browser(link, w, log)
		return state{}
	}
	return state{done: done, opened: opened}
}

// show — повторный запуск. Окно открыто: новый процесс Chromium с тем же профилем лишь выводит его
// вперёд и сразу выходит — его не отслеживаем и за провал не считаем. Окна нет — открываем.
func show(link string, w Window, cur state, now func() time.Time, log *slog.Logger) state {
	if cur.done == nil {
		return open(link, w, now, log)
	}
	if _, err := w.OpenApp(link); err != nil {
		log.Warn("не удалось вывести окно вперёд", "err", err)
	}
	return cur
}

func browser(link string, w Window, log *slog.Logger) {
	if err := w.OpenInOS(link, false); err != nil {
		log.Error("не удалось открыть браузер", "err", err)
	}
}
