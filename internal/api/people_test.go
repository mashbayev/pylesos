// Файл: internal/api/people_test.go
package api

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"pylesos/internal/events"
)

func TestPeopleRoutes(t *testing.T) {
	_, ts, fs, _ := start(t, fakeJobs{})
	if resp := do(t, http.MethodGet, ts.URL+"/api/screens/people?tab=unlinked&q=%D0%B8%D0%B2%D0%B0%D0%BD&id=3", "секрет", ""); resp.StatusCode != http.StatusOK ||
		fs.saved["tab"] != "unlinked" || fs.saved["q"] != "иван" || fs.saved["id"] != "3" {
		t.Fatalf("экран: %v", fs.saved)
	}
	for body, want := range map[string]map[string]string{
		`merge {"a":1,"b":2}`:           {"merge": "1,2"},
		`reject {"a":1,"b":2}`:          {"reject": "1,2"},
		`link {"mention":9,"person":0}`: {"link": "9,0"},
	} {
		var action, json string
		for i := range body {
			if body[i] == ' ' {
				action, json = body[:i], body[i+1:]
				break
			}
		}
		if resp := do(t, http.MethodPost, ts.URL+"/api/people/"+action, "секрет", json); resp.StatusCode != http.StatusOK {
			t.Fatalf("%s: %d", action, resp.StatusCode)
		}
		for k, v := range want {
			if fs.saved[k] != v {
				t.Fatalf("%s: %v", action, fs.saved)
			}
		}
	}
	if resp := do(t, http.MethodPost, ts.URL+"/api/people/unknown", "секрет", `{}`); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("неизвестное действие: %d", resp.StatusCode)
	}
}

func TestAnalyzeSource(t *testing.T) {
	var did string
	s, err := New(Deps{Token: "секрет", Static: fstest.MapFS{}, Bus: events.NewBus(), Screens: &fakeScreens{},
		Jobs: fakeJobs{}, Sources: fakeCollector{did: &did}, Log: slog.New(slog.NewJSONHandler(io.Discard, nil))})
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
	if resp := do(t, http.MethodPost, ts.URL+"/api/sources/5/analyze", "секрет", `{}`); resp.StatusCode != http.StatusOK || did != "analyze:5" {
		t.Fatalf("разобрать: %d %q", resp.StatusCode, did)
	}
	if resp := do(t, http.MethodPost, ts.URL+"/api/sources/6/export", "секрет", `{}`); resp.StatusCode != http.StatusOK || did != "export:6" {
		t.Fatalf("в Excel: %d %q", resp.StatusCode, did)
	}
	if resp := do(t, http.MethodPost, ts.URL+"/api/sources/5/delete", "секрет", `{}`); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("неизвестное действие: %d", resp.StatusCode)
	}
}

func TestTableRoutes(t *testing.T) {
	_, ts, fs, _ := start(t, fakeJobs{})
	if resp := do(t, http.MethodGet, ts.URL+"/api/screens/table", "секрет", ""); resp.StatusCode != http.StatusOK {
		t.Fatalf("экран: %d", resp.StatusCode)
	}
	resp := do(t, http.MethodPost, ts.URL+"/api/table", "секрет", `{"columns":[{"name":"ИИН","type":"iin","hint":""}],"include_queries":true}`)
	if resp.StatusCode != http.StatusOK || fs.saved["col"] != "ИИН" || fs.saved["inc"] != "true" {
		t.Fatalf("сохранение: %d %v", resp.StatusCode, fs.saved)
	}
}
