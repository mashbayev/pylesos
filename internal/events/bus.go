// Файл: internal/events/bus.go
// Package events — оповещение открытых окон об изменениях (поток SSE).
// События — только сигнал «перечитай экран», поэтому потерять лишнее не страшно.
package events

import "sync"

// Типы событий.
const (
	JobsChanged = "jobs_changed"
	BotChanged  = "bot_changed"
)

// Event — какие экраны окну перечитать (решает бэкенд, фронт только выполняет).
type Event struct {
	Type    string   `json:"type"`
	Screens []string `json:"screens"`
}

// Jobs — изменились задания или собранные данные: перечитать «Задания» и «Источники».
// Разбор (tg_analyze) пополняет людей — экран «Люди» тоже перечитывается.
func Jobs() Event { return Event{Type: JobsChanged, Screens: []string{"jobs", "home", "people"}} }

// Bot — сменился статус бота: перечитать «Telegram».
func Bot() Event { return Event{Type: BotChanged, Screens: []string{"telegram"}} }

const subscriberBuffer = 16

type Bus struct {
	mu   sync.Mutex
	subs map[chan Event]struct{}
}

func NewBus() *Bus { return &Bus{subs: map[chan Event]struct{}{}} }

// Subscribe возвращает канал событий и функцию отписки (её можно вызывать повторно).
func (b *Bus) Subscribe() (<-chan Event, func()) {
	ch := make(chan Event, subscriberBuffer)
	b.mu.Lock()
	b.subs[ch] = struct{}{}
	b.mu.Unlock()
	var once sync.Once
	return ch, func() {
		once.Do(func() {
			b.mu.Lock()
			delete(b.subs, ch)
			b.mu.Unlock()
			close(ch)
		})
	}
}

// Publish рассылает событие; подписчику с полным буфером событие не достаётся — программа не ждёт.
func (b *Bus) Publish(e Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.subs {
		select {
		case ch <- e:
		default:
		}
	}
}
