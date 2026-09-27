// Файл: internal/api/shutdown_test.go
package api

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"testing"
	"testing/fstest"
	"time"

	"pylesos/internal/events"
)

// Открытое окно держит поток /api/events. Штатный выход не должен упираться в таймаут Shutdown.
func TestShutdownWithOpenEventStream(t *testing.T) {
	s, err := New(Deps{Token: "секрет", Static: fstest.MapFS{}, Bus: events.NewBus(), Screens: &fakeScreens{},
		Jobs: fakeJobs{}, Sources: fakeCollector{}, Log: slog.New(slog.NewJSONHandler(io.Discard, nil))})
	if err != nil {
		t.Fatal(err)
	}
	ln, err := s.Listen()
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: s.Handler(), ReadHeaderTimeout: time.Second}
	go func() {
		if err := srv.Serve(ln); err != http.ErrServerClosed {
			t.Errorf("Serve: %v", err)
		}
	}()
	resp, err := http.Get("http://" + ln.Addr().String() + "/api/events?t=секрет")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Errorf("закрыть поток: %v", err)
		}
	}()
	s.StopStreams()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown при открытом потоке событий: %v", err)
	}
}
