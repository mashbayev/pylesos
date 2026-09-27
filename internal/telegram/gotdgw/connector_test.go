// Файл: internal/telegram/gotdgw/connector_test.go
package gotdgw

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"pylesos/internal/telegram/domain"
)

// fakeDial — «клиент» с управляемым подключением: ready закрывается, когда тест разрешит.
type fakeDial struct {
	mu      sync.Mutex
	dials   atomic.Int32
	allow   chan struct{} // закрыть — подключение удалось
	failErr error         // подключение сразу проваливается
}

func (f *fakeDial) dial(context.Context) (int, runFunc, error) {
	n := f.dials.Add(1)
	run := func(ctx context.Context, onReady func()) error {
		if f.failErr != nil {
			return f.failErr
		}
		select {
		case <-f.allow:
			onReady()
			<-ctx.Done()
			return ctx.Err()
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return int(n), run, nil
}

func newTestConnector(f *fakeDial, timeout time.Duration) *connector[int] {
	return newConnector[int](context.Background(), f.dial, timeout)
}

func TestConnectorReady(t *testing.T) {
	f := &fakeDial{allow: make(chan struct{})}
	close(f.allow)
	c := newTestConnector(f, time.Second)
	for i := 0; i < 3; i++ {
		if got, err := c.get(context.Background()); err != nil || got != 1 {
			t.Fatalf("клиент %d, %v", got, err)
		}
	}
	c.reset()
	if got, err := c.get(context.Background()); err != nil || got != 2 {
		t.Fatalf("после сброса — новое подключение: %d %v", got, err)
	}
}

// Без сети: каждый вызов ждёт не дольше таймаута и не блокирует соседей (ревью M2, I3).
func TestConnectorNoNetworkTimesOutWithoutBlocking(t *testing.T) {
	f := &fakeDial{allow: make(chan struct{})}
	c := newTestConnector(f, 100*time.Millisecond)
	start := time.Now()
	var wg sync.WaitGroup
	errs := make([]error, 3)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = c.get(context.Background())
		}(i)
	}
	wg.Wait()
	if time.Since(start) > 500*time.Millisecond {
		t.Fatalf("вызовы ждали друг друга: %v", time.Since(start))
	}
	for _, err := range errs {
		if !errors.Is(err, domain.ErrUnavailable) {
			t.Fatalf("ждали «нет связи»: %v", err)
		}
	}
	if f.dials.Load() != 1 {
		t.Fatalf("подключений %d — должно идти одно в фоне", f.dials.Load())
	}
	close(f.allow) // связь появилась
	if got, err := c.get(context.Background()); err != nil || got != 1 {
		t.Fatalf("после появления связи: %d %v", got, err)
	}
}

func TestConnectorFailureRetriesAndContextCancel(t *testing.T) {
	f := &fakeDial{failErr: errors.New("отказ")}
	c := newTestConnector(f, time.Second)
	if _, err := c.get(context.Background()); err == nil {
		t.Fatal("ждали ошибку подключения")
	}
	f.failErr, f.allow = nil, make(chan struct{})
	close(f.allow)
	if got, err := c.get(context.Background()); err != nil || got != 2 {
		t.Fatalf("после отказа — новая попытка: %d %v", got, err)
	}
	c.reset()
	f.allow = make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.get(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("отменённый вызов: %v", err)
	}
}
