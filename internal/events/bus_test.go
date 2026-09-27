// Файл: internal/events/bus_test.go
package events

import (
	"testing"
	"time"
)

func TestSubscribeReceivesAndUnsubscribeCloses(t *testing.T) {
	bus := NewBus()
	ch, unsubscribe := bus.Subscribe()
	bus.Publish(Event{Type: JobsChanged})
	select {
	case e := <-ch:
		if e.Type != JobsChanged {
			t.Fatalf("событие %q", e.Type)
		}
	case <-time.After(time.Second):
		t.Fatal("событие не пришло")
	}
	unsubscribe()
	if _, ok := <-ch; ok {
		t.Fatal("канал не закрыт после отписки")
	}
	unsubscribe() // повторная отписка безопасна
}

func TestPublishDoesNotBlockOnSlowSubscriber(t *testing.T) {
	bus := NewBus()
	_, unsubscribe := bus.Subscribe() // никто не читает
	defer unsubscribe()
	done := make(chan struct{})
	go func() {
		for i := 0; i < 1000; i++ {
			bus.Publish(Event{Type: JobsChanged})
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Publish заблокировался на медленном подписчике")
	}
}
