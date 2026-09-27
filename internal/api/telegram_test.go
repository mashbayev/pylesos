// Файл: internal/api/telegram_test.go
package api

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"pylesos/internal/events"
	jobsdomain "pylesos/internal/jobs/domain"
	"pylesos/internal/screens"
	"pylesos/internal/texts"
)

func TestTelegramRoutes(t *testing.T) {
	_, ts, fs, _ := start(t, fakeJobs{})
	if resp := do(t, http.MethodGet, ts.URL+"/api/screens/telegram", "секрет", ""); resp.StatusCode != http.StatusOK {
		t.Fatalf("экран: %d", resp.StatusCode)
	}
	do(t, http.MethodPost, ts.URL+"/api/telegram/submit", "секрет", `{"values":{"phone":"+77011234567"}}`)
	if fs.saved["phone"] != "+77011234567" {
		t.Fatalf("шаг входа не передан: %v", fs.saved)
	}
	resp := do(t, http.MethodPost, ts.URL+"/api/telegram/groups", "секрет", `{"key":"channel:1:2"}`)
	var s screens.TelegramScreen
	if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
		t.Fatal(err)
	}
	if fs.saved["key"] != "channel:1:2" || s.Message != texts.TgGroupAdded {
		t.Fatalf("добавление группы: %v %+v", fs.saved, s)
	}
	if resp := do(t, http.MethodPost, ts.URL+"/api/telegram/refresh", "секрет", `{}`); resp.StatusCode != http.StatusOK {
		t.Fatalf("обновление: %d", resp.StatusCode)
	}
	if resp := do(t, http.MethodPost, ts.URL+"/api/sources/5/collect", "секрет", `{}`); resp.StatusCode != http.StatusOK {
		t.Fatalf("собрать: %d", resp.StatusCode)
	}
	resp = do(t, http.MethodPost, ts.URL+"/api/telegram/keys", "секрет", `{}`)
	var keys screens.TelegramScreen
	if err := json.NewDecoder(resp.Body).Decode(&keys); err != nil || keys.Hint != texts.TgKeysHint {
		t.Fatalf("изменить ключи: %d %+v %v", resp.StatusCode, keys, err)
	}
	do(t, http.MethodPost, ts.URL+"/api/telegram/bot", "секрет", `{"token":"123:abc"}`)
	if fs.saved["token"] != "123:abc" {
		t.Fatalf("токен бота не передан: %v", fs.saved)
	}
}

func TestCollectBusySource(t *testing.T) {
	s, err := New(Deps{Token: "секрет", Static: fstest.MapFS{}, Bus: events.NewBus(), Screens: &fakeScreens{},
		Jobs: fakeJobs{}, Sources: fakeCollector{err: jobsdomain.ErrSourceBusy}, Log: slog.New(slog.NewJSONHandler(io.Discard, nil))})
	if err != nil {
		t.Fatal(err)
	}
	ln, err := s.Listen()
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewUnstartedServer(s.Handler())
	ts.Listener = ln
	ts.Start()
	defer ts.Close()
	resp := do(t, http.MethodPost, ts.URL+"/api/sources/5/collect", "секрет", `{}`)
	var body struct{ Error string }
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusConflict || body.Error != texts.ErrSourceBusy {
		t.Fatalf("%d %q", resp.StatusCode, body.Error)
	}
}
