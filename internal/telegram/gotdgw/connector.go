// Файл: internal/telegram/gotdgw/connector.go
package gotdgw

import (
	"context"
	"fmt"
	"sync"
	"time"

	"pylesos/internal/telegram/domain"
)

// runFunc держит соединение, пока не отменён ctx; onReady — соединение готово к запросам.
type runFunc func(ctx context.Context, onReady func()) error

// dialFunc создаёт клиента и функцию, держащую его соединение.
type dialFunc[C any] func(ctx context.Context) (C, runFunc, error)

// attempt — одна попытка соединения. err пишется до закрытия ready.
type attempt[C any] struct {
	client C
	stop   context.CancelFunc
	ready  chan struct{}
	once   sync.Once
	err    error
}

// failed — попытка уже закончилась ошибкой (её место займёт новая, не дожидаясь фоновой уборки).
func (a *attempt[C]) failed() bool {
	select {
	case <-a.ready:
		return a.err != nil
	default:
		return false
	}
}

func (a *attempt[C]) done(err error) {
	a.once.Do(func() {
		a.err = err
		close(a.ready)
	})
}

// connector — одно соединение на программу. Подключение идёт в фоне и не держит блокировку,
// вызов ждёт его не дольше timeout (ревью M2, I3): без сети — domain.ErrUnavailable, а не вечное ожидание.
type connector[C any] struct {
	base    context.Context // отменяется при выходе из программы
	dial    dialFunc[C]
	timeout time.Duration

	mu  sync.Mutex
	cur *attempt[C]
}

func newConnector[C any](base context.Context, dial dialFunc[C], timeout time.Duration) *connector[C] {
	return &connector[C]{base: base, dial: dial, timeout: timeout}
}

func (c *connector[C]) get(ctx context.Context) (C, error) {
	var zero C
	a, err := c.current(ctx)
	if err != nil {
		return zero, err
	}
	timer := time.NewTimer(c.timeout)
	defer timer.Stop()
	select {
	case <-a.ready:
		if a.err != nil {
			return zero, fmt.Errorf("подключиться к Telegram: %w", a.err)
		}
		return a.client, nil
	case <-ctx.Done():
		return zero, ctx.Err()
	case <-timer.C:
		return zero, domain.ErrUnavailable
	}
}

// current — идущая попытка или новая; блокировка держится только на время запуска.
func (c *connector[C]) current(ctx context.Context) (*attempt[C], error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cur != nil && !c.cur.failed() {
		return c.cur, nil
	}
	client, run, err := c.dial(ctx)
	if err != nil {
		return nil, err
	}
	runCtx, stop := context.WithCancel(c.base)
	a := &attempt[C]{client: client, stop: stop, ready: make(chan struct{})}
	c.cur = a
	go c.run(runCtx, a, run)
	return a, nil
}

func (c *connector[C]) run(ctx context.Context, a *attempt[C], run runFunc) {
	err := run(ctx, func() { a.done(nil) })
	if err == nil {
		err = fmt.Errorf("соединение закрыто")
	}
	a.done(err)
	c.mu.Lock()
	if c.cur == a {
		c.cur = nil // следующий вызов подключится заново
	}
	c.mu.Unlock()
	a.stop()
}

// reset разрывает соединение: следующий вызов подключится заново (сменились ключи API).
func (c *connector[C]) reset() {
	c.mu.Lock()
	a := c.cur
	c.cur = nil
	c.mu.Unlock()
	if a != nil {
		a.stop()
	}
}
