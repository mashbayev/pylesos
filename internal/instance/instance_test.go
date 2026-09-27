// Файл: internal/instance/instance_test.go
package instance

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"pylesos/internal/api"
)

func TestShowRunning(t *testing.T) {
	var shown atomic.Bool // пишется в горутине сервера — тест гоняется с -race
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/show" && r.Header.Get(api.TokenHeader) == "секрет" {
			shown.Store(true)
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusForbidden)
	}))
	defer ts.Close()
	path := filepath.Join(t.TempDir(), "instance.json")
	if ShowRunning(context.Background(), ts.Client(), path) {
		t.Fatal("нет файла — не должно считаться запущенным")
	}
	if err := Write(path, ts.URL+"/?t=секрет"); err != nil {
		t.Fatal(err)
	}
	if !ShowRunning(context.Background(), ts.Client(), path) || !shown.Load() {
		t.Fatal("запущенная программа не показала окно")
	}
	if err := os.WriteFile(path, []byte("мусор"), 0o600); err != nil {
		t.Fatal(err)
	}
	if ShowRunning(context.Background(), ts.Client(), path) {
		t.Fatal("испорченный файл — не должно считаться запущенным")
	}
}

// Первая копия ещё стартует (файла экземпляра нет) — вторая ждёт и дожидается её.
func TestShowRunningWithinWaitsForStartingInstance(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	defer ts.Close()
	path := filepath.Join(t.TempDir(), "instance.json")
	go func() {
		time.Sleep(150 * time.Millisecond)
		if err := Write(path, ts.URL+"/?t=секрет"); err != nil {
			t.Errorf("записать файл экземпляра: %v", err)
		}
	}()
	if !ShowRunningWithin(context.Background(), ts.Client(), path, 3*time.Second, 50*time.Millisecond) {
		t.Fatal("не дождались стартующей копии")
	}
	if ShowRunningWithin(context.Background(), ts.Client(), filepath.Join(t.TempDir(), "нет.json"), 200*time.Millisecond, 50*time.Millisecond) {
		t.Fatal("копии нет — должно вернуть false по таймауту")
	}
}
