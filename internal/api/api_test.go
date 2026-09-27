// Файл: internal/api/api_test.go
package api

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	docdomain "pylesos/internal/docforms/domain"
	"pylesos/internal/events"
	jobsdomain "pylesos/internal/jobs/domain"
	jobsservice "pylesos/internal/jobs/service"
	"pylesos/internal/screens"
	"pylesos/internal/tables"
	"pylesos/internal/texts"
)

type fakeScreens struct{ saved map[string]string }

func (f *fakeScreens) Layout() screens.Layout { return screens.Layout{AppName: texts.AppName} }
func (f *fakeScreens) Home(context.Context) (screens.HomeScreen, error) {
	return screens.HomeScreen{Title: texts.HomeTitle}, nil
}
func (f *fakeScreens) Jobs(context.Context) (screens.JobsScreen, error) {
	return screens.JobsScreen{Title: texts.JobsTitle, Jobs: []screens.JobRow{}}, nil
}
func (f *fakeScreens) Settings(context.Context) (screens.SettingsScreen, error) {
	return screens.SettingsScreen{Title: texts.SettingsTitle}, nil
}
func (f *fakeScreens) SaveSettings(_ context.Context, in map[string]string) (screens.SettingsScreen, error) {
	f.saved = in
	return screens.SettingsScreen{Title: texts.SettingsTitle, Message: texts.SettingsSaved}, nil
}

func (f *fakeScreens) Telegram(context.Context) (screens.TelegramScreen, error) {
	return screens.TelegramScreen{Title: texts.TgTitle}, nil
}
func (f *fakeScreens) TelegramSubmit(_ context.Context, v map[string]string) (screens.TelegramScreen, error) {
	f.saved = v
	return screens.TelegramScreen{Title: texts.TgTitle}, nil
}
func (f *fakeScreens) TelegramAddGroup(_ context.Context, key string) (screens.TelegramScreen, error) {
	f.saved = map[string]string{"key": key}
	return screens.TelegramScreen{Message: texts.TgGroupAdded}, nil
}
func (f *fakeScreens) TelegramEditKeys(context.Context) (screens.TelegramScreen, error) {
	return screens.TelegramScreen{Hint: texts.TgKeysHint}, nil
}
func (f *fakeScreens) TelegramBot(_ context.Context, token string) (screens.TelegramScreen, error) {
	f.saved = map[string]string{"token": token}
	return screens.TelegramScreen{Message: texts.TgBotSaved}, nil
}
func (f *fakeScreens) Forms(_ context.Context, id int64) (screens.FormsScreen, error) {
	f.saved = map[string]string{"open": strconv.FormatInt(id, 10)}
	return screens.FormsScreen{Title: texts.FormsTitle}, nil
}
func (f *fakeScreens) FormsCreate(_ context.Context, name, fileName string, data []byte) (screens.FormsScreen, error) {
	f.saved = map[string]string{"name": name, "file": fileName, "data": string(data)}
	return screens.FormsScreen{Message: texts.FormsCreated}, nil
}
func (f *fakeScreens) FormsSetZone(_ context.Context, id int64, key string, z tables.Zone) (screens.FormsScreen, error) {
	f.saved = map[string]string{"id": strconv.FormatInt(id, 10), "key": key, "x": strconv.FormatFloat(z.X, 'f', 2, 64)}
	return screens.FormsScreen{Message: texts.FormsSaved}, nil
}
func (f *fakeScreens) FormsClearZone(_ context.Context, id int64, key string) (screens.FormsScreen, error) {
	f.saved = map[string]string{"clear": key}
	return screens.FormsScreen{}, nil
}
func (f *fakeScreens) FormsRename(_ context.Context, id int64, name string) (screens.FormsScreen, error) {
	f.saved = map[string]string{"rename": name}
	return screens.FormsScreen{}, nil
}
func (f *fakeScreens) FormsDelete(_ context.Context, id int64) (screens.FormsScreen, error) {
	f.saved = map[string]string{"delete": strconv.FormatInt(id, 10)}
	return screens.FormsScreen{}, nil
}
func (f *fakeScreens) People(_ context.Context, q screens.PeopleQuery) (screens.PeopleScreen, error) {
	f.saved = map[string]string{"tab": q.Tab, "q": q.Query, "id": strconv.FormatInt(q.Person, 10)}
	return screens.PeopleScreen{Title: texts.PeopleTitle}, nil
}
func (f *fakeScreens) PeopleMerge(_ context.Context, a, b int64) (screens.PeopleScreen, error) {
	f.saved = map[string]string{"merge": strconv.FormatInt(a, 10) + "," + strconv.FormatInt(b, 10)}
	return screens.PeopleScreen{Message: texts.PeopleMerged}, nil
}
func (f *fakeScreens) PeopleReject(_ context.Context, a, b int64) (screens.PeopleScreen, error) {
	f.saved = map[string]string{"reject": strconv.FormatInt(a, 10) + "," + strconv.FormatInt(b, 10)}
	return screens.PeopleScreen{}, nil
}
func (f *fakeScreens) PeopleLink(_ context.Context, m, p int64) (screens.PeopleScreen, error) {
	f.saved = map[string]string{"link": strconv.FormatInt(m, 10) + "," + strconv.FormatInt(p, 10)}
	return screens.PeopleScreen{}, nil
}
func (f *fakeScreens) Table(context.Context) (screens.TableScreen, error) {
	return screens.TableScreen{Title: texts.TableTitle}, nil
}
func (f *fakeScreens) TableSave(_ context.Context, cols []screens.TableColumn, inc bool) (screens.TableScreen, error) {
	f.saved = map[string]string{"col": cols[0].Name, "inc": strconv.FormatBool(inc)}
	return screens.TableScreen{Message: texts.TableSaved}, nil
}
func (f *fakeScreens) TelegramRefresh(context.Context) (screens.TelegramScreen, error) {
	return screens.TelegramScreen{Title: texts.TgTitle}, nil
}

type fakeSamples struct{}

func (fakeSamples) Sample(_ context.Context, id int64) ([]byte, error) {
	if id == 404 {
		return nil, docdomain.ErrNotFound
	}
	return []byte("jpeg-" + strconv.FormatInt(id, 10)), nil
}

type fakeCollector struct {
	err error
	did *string
}

func (f fakeCollector) Collect(_ context.Context, id int64) error { return f.note("collect", id) }
func (f fakeCollector) Analyze(_ context.Context, id int64) error { return f.note("analyze", id) }
func (f fakeCollector) Export(_ context.Context, id int64) error  { return f.note("export", id) }
func (f fakeCollector) note(what string, id int64) error {
	if f.did != nil {
		*f.did = what + ":" + strconv.FormatInt(id, 10)
	}
	return f.err
}

type fakeJobs struct{ err error }

func (f fakeJobs) Pause(context.Context, int64) error  { return f.err }
func (f fakeJobs) Resume(context.Context, int64) error { return f.err }
func (f fakeJobs) Cancel(context.Context, int64) error { return f.err }

func start(t *testing.T, jobs JobActions) (*Server, *httptest.Server, *fakeScreens, *events.Bus) {
	t.Helper()
	fs := &fakeScreens{}
	bus := events.NewBus()
	s, err := New(Deps{Token: "секрет", Static: fstest.MapFS{"index.html": {Data: []byte("<html>")}},
		Bus: bus, Screens: fs, Jobs: jobs, Sources: fakeCollector{}, Samples: fakeSamples{}, Log: slog.New(slog.NewJSONHandler(io.Discard, nil))})
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
	t.Cleanup(ts.Close)
	return s, ts, fs, bus
}

func do(t *testing.T, method, url, token, body string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		req.Header.Set(TokenHeader, token)
	}
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := resp.Body.Close(); err != nil {
			t.Errorf("закрыть ответ: %v", err)
		}
	})
	return resp
}

func TestGuard(t *testing.T) {
	s, ts, _, _ := start(t, fakeJobs{})
	if !strings.HasPrefix(s.URL(), "http://127.0.0.1:") || !strings.Contains(s.URL(), "t=") {
		t.Fatalf("адрес окна %q", s.URL())
	}
	if resp := do(t, http.MethodGet, ts.URL+"/api/screens/home", "", ""); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("без токена: %d", resp.StatusCode)
	}
	if resp := do(t, http.MethodGet, ts.URL+"/api/screens/home", "секрет", ""); resp.StatusCode != http.StatusOK {
		t.Fatalf("с токеном: %d", resp.StatusCode)
	}
	req, err := http.NewRequest(http.MethodGet, ts.URL+"/api/screens/home", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Host = "evil.example:80"
	req.Header.Set(TokenHeader, "секрет")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if err := resp.Body.Close(); err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("чужой Host: %d", resp.StatusCode)
	}
	plain, err := http.NewRequest(http.MethodPost, ts.URL+"/api/settings", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	plain.Header.Set(TokenHeader, "секрет")
	plain.Header.Set("Content-Type", "text/plain")
	resp, err = http.DefaultClient.Do(plain)
	if err != nil {
		t.Fatal(err)
	}
	if err := resp.Body.Close(); err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusUnsupportedMediaType {
		t.Fatalf("POST не JSON: %d", resp.StatusCode)
	}
	if resp := do(t, http.MethodGet, ts.URL+"/", "", ""); resp.StatusCode != http.StatusOK {
		t.Fatalf("статика без токена должна открываться: %d", resp.StatusCode)
	}
}

func TestSettingsAndJobActions(t *testing.T) {
	_, ts, fs, _ := start(t, fakeJobs{})
	resp := do(t, http.MethodPost, ts.URL+"/api/settings", "секрет", `{"values":{"exports_dir":"/data"}}`)
	var screen screens.SettingsScreen
	if err := json.NewDecoder(resp.Body).Decode(&screen); err != nil {
		t.Fatal(err)
	}
	if screen.Message != texts.SettingsSaved || fs.saved["exports_dir"] != "/data" {
		t.Fatalf("экран %+v, сохранено %v", screen, fs.saved)
	}
	if resp := do(t, http.MethodPost, ts.URL+"/api/settings", "секрет", `{не json`); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("кривой JSON: %d", resp.StatusCode)
	}
	if resp := do(t, http.MethodPost, ts.URL+"/api/jobs/7/pause", "секрет", `{}`); resp.StatusCode != http.StatusOK {
		t.Fatalf("пауза: %d", resp.StatusCode)
	}
	if resp := do(t, http.MethodPost, ts.URL+"/api/jobs/7/explode", "секрет", `{}`); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("неизвестное действие: %d", resp.StatusCode)
	}
}

func TestJobActionErrors(t *testing.T) {
	cases := []struct {
		err  error
		code int
		text string
	}{
		{jobsservice.ErrNotRunning, http.StatusConflict, texts.ErrActionUnavailable},
		{jobsdomain.ErrSourceBusy, http.StatusConflict, texts.ErrSourceBusy},
		{jobsdomain.ErrNotFound, http.StatusNotFound, texts.ErrJobNotFound},
		{io.ErrUnexpectedEOF, http.StatusInternalServerError, texts.ErrInternal},
	}
	for _, c := range cases {
		_, ts, _, _ := start(t, fakeJobs{err: c.err})
		resp := do(t, http.MethodPost, ts.URL+"/api/jobs/1/resume", "секрет", `{}`)
		var body struct{ Error string }
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != c.code || body.Error != c.text {
			t.Fatalf("%v: %d %q", c.err, resp.StatusCode, body.Error)
		}
	}
}

func TestEventsStream(t *testing.T) {
	_, ts, _, bus := start(t, fakeJobs{})
	resp := do(t, http.MethodGet, ts.URL+"/api/events?t=секрет", "", "")
	if resp.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("тип %q", resp.Header.Get("Content-Type"))
	}
	lines := make(chan string, 4)
	go func() {
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			lines <- sc.Text()
		}
	}()
	deadline := time.After(2 * time.Second)
	for {
		bus.Publish(events.Jobs())
		select {
		case line := <-lines:
			if line == `data: {"type":"jobs_changed","screens":["jobs","home","people"]}` {
				return
			}
		case <-time.After(50 * time.Millisecond):
		case <-deadline:
			t.Fatal("событие не пришло в поток")
		}
	}
}

// Отказ защиты — тоже готовый русский текст (П22), а не «forbidden».
func TestGuardErrorsInRussian(t *testing.T) {
	_, ts, _, _ := start(t, fakeJobs{})
	resp := do(t, http.MethodGet, ts.URL+"/api/screens/home", "чужой", "")
	var body struct{ Error string }
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("ответ не JSON: %v", err)
	}
	if resp.StatusCode != http.StatusForbidden || body.Error != texts.ErrForbidden {
		t.Fatalf("%d %q", resp.StatusCode, body.Error)
	}
}
