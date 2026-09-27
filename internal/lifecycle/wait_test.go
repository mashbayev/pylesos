// Файл: internal/lifecycle/wait_test.go
package lifecycle

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"
)

// fakeWindow — окно программы: каждый OpenApp — новый «процесс Chromium» с каналом закрытия.
type fakeWindow struct {
	mu      sync.Mutex
	fail    bool
	dones   []chan struct{}
	browser int
}

func (f *fakeWindow) OpenApp(string) (<-chan struct{}, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail {
		return nil, errors.New("нет Chromium")
	}
	ch := make(chan struct{})
	f.dones = append(f.dones, ch)
	return ch, nil
}

func (f *fakeWindow) OpenInOS(string, bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.browser++
	return nil
}

func (f *fakeWindow) counts() (int, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.dones), f.browser
}

func (f *fakeWindow) closeWindow(i int) {
	f.mu.Lock()
	ch := f.dones[i]
	f.mu.Unlock()
	close(ch)
}

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time      { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *clock) add(d time.Duration) { c.mu.Lock(); c.t = c.t.Add(d); c.mu.Unlock() }

type harness struct {
	win  *fakeWindow
	clk  *clock
	quit chan struct{}
	show chan struct{}
	done chan error
}

func run(t *testing.T, win *fakeWindow) harness {
	t.Helper()
	h := harness{win: win, clk: &clock{t: time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)},
		quit: make(chan struct{}), show: make(chan struct{}), done: make(chan error, 1)}
	sig := Signals{Quit: h.quit, Show: h.show, ServeErr: make(chan error)}
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	go func() { h.done <- WaitForExit(context.Background(), "http://x", win, sig, h.clk.now, log) }()
	return h
}

// until ждёт, пока окон/вкладок станет не меньше заданного.
func (h harness) until(t *testing.T, windows, tabs int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		w, b := h.win.counts()
		if w >= windows && b >= tabs {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("окон %d (ждали %d), вкладок %d (ждали %d)", w, windows, b, tabs)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func (h harness) exited(t *testing.T) bool {
	t.Helper()
	select {
	case err := <-h.done:
		if err != nil {
			t.Fatalf("WaitForExit: %v", err)
		}
		return true
	case <-time.After(100 * time.Millisecond):
		return false
	}
}

func TestClosingWindowExits(t *testing.T) {
	h := run(t, &fakeWindow{})
	h.until(t, 1, 0)
	h.clk.add(time.Minute)
	h.win.closeWindow(0)
	if !h.exited(t) {
		t.Fatal("закрытие окна не завершило программу")
	}
}

// Повторный запуск при открытом окне: окно выводится вперёд, лишней вкладки нет, закрытие окна по-прежнему выход.
func TestShowWhileWindowOpen(t *testing.T) {
	h := run(t, &fakeWindow{})
	h.until(t, 1, 0)
	h.show <- struct{}{}
	h.until(t, 2, 0)
	h.win.closeWindow(1) // процесс Chromium передал --app уже открытому окну и сразу вышел
	if h.exited(t) {
		t.Fatal("выход из-за процесса, который лишь показал окно")
	}
	if _, tabs := h.win.counts(); tabs != 0 {
		t.Fatalf("открыта лишняя вкладка браузера: %d", tabs)
	}
	h.clk.add(time.Minute)
	h.win.closeWindow(0)
	if !h.exited(t) {
		t.Fatal("закрытие окна после повторного запуска не завершило программу")
	}
}

// Окно сразу закрылось (нет Chromium-окна) — страница в браузере, выход только кнопкой; «показать» открывает окно снова.
func TestQuickCloseFallsBackToBrowser(t *testing.T) {
	h := run(t, &fakeWindow{})
	h.until(t, 1, 0)
	h.clk.add(time.Second)
	h.win.closeWindow(0)
	h.until(t, 1, 1)
	if h.exited(t) {
		t.Fatal("программа завершилась, хотя окно не открылось")
	}
	h.show <- struct{}{}
	h.until(t, 2, 1)
	h.clk.add(time.Minute)
	h.win.closeWindow(1)
	if !h.exited(t) {
		t.Fatal("закрытие нового окна не завершило программу")
	}
}

func TestNoChromiumWaitsForQuit(t *testing.T) {
	h := run(t, &fakeWindow{fail: true})
	h.until(t, 0, 1)
	if h.exited(t) {
		t.Fatal("выход без команды")
	}
	close(h.quit)
	if !h.exited(t) {
		t.Fatal("«Выход» не завершил программу")
	}
}
