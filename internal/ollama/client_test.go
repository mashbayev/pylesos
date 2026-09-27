// Файл: internal/ollama/client_test.go
package ollama

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"pylesos/internal/llm"
)

func TestRespondSendsSystemTextAndImages(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			t.Errorf("путь %s", r.URL.Path)
		}
		raw, _ := io.ReadAll(r.Body)
		json.Unmarshal(raw, &body)
		io.WriteString(w, `{"message":{"role":"assistant","content":"Қазақстан"},"done":true,"done_reason":"stop"}`)
	}))
	defer srv.Close()
	got, err := New(srv.URL).Respond(context.Background(), llm.Request{Model: "gemma4:e4b", System: "SYS", Text: "Страница 1.", Images: [][]byte{{1, 2, 3}}})
	if err != nil || got != "Қазақстан" {
		t.Fatalf("got %q err %v", got, err)
	}
	if body["model"] != "gemma4:e4b" || body["stream"] != false || body["think"] != false {
		t.Fatalf("body %v", body)
	}
	msgs := body["messages"].([]any)
	sys, user := msgs[0].(map[string]any), msgs[1].(map[string]any)
	if sys["role"] != "system" || sys["content"] != "SYS" || user["content"] != "Страница 1." {
		t.Fatalf("messages %v", msgs)
	}
	if imgs := user["images"].([]any); len(imgs) != 1 || imgs[0] != base64.StdEncoding.EncodeToString([]byte{1, 2, 3}) {
		t.Fatalf("images %v", user["images"])
	}
	if opts := body["options"].(map[string]any); opts["temperature"] != 0.0 {
		t.Fatalf("options %v", opts)
	}
}

func TestRespondErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		io.WriteString(w, `{"error":"model \"x\" not found, try pulling it first"}`)
	}))
	defer srv.Close()
	_, err := New(srv.URL).Respond(context.Background(), llm.Request{Model: "x", Text: "t"})
	if !errors.Is(err, llm.ErrTurnFailed) {
		t.Fatalf("err %v", err)
	}

	empty := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"message":{"content":"  "},"done":true}`)
	}))
	defer empty.Close()
	if _, err := New(empty.URL).Respond(context.Background(), llm.Request{Model: "m", Text: "t"}); !errors.Is(err, llm.ErrTurnFailed) {
		t.Fatalf("пустой ответ: %v", err)
	}

	down := httptest.NewServer(http.NotFoundHandler())
	url := down.URL
	down.Close()
	if _, err := New(url).Respond(context.Background(), llm.Request{Model: "m", Text: "t"}); !errors.Is(err, ErrNotRunning) {
		t.Fatalf("Ollama не запущена: %v", err)
	}
}

func TestVisionModels(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			io.WriteString(w, `{"models":[{"name":"qwen2.5-coder:7b"},{"name":"gemma4:e4b"},{"name":"gemma4:12b"}]}`)
		case "/api/show":
			var req struct{ Model string }
			json.NewDecoder(r.Body).Decode(&req)
			caps := `["completion"]`
			if req.Model != "qwen2.5-coder:7b" {
				caps = `["completion","vision"]`
			}
			io.WriteString(w, `{"capabilities":`+caps+`}`)
		case "/api/version":
			io.WriteString(w, `{"version":"0.32.3"}`)
		}
	}))
	defer srv.Close()
	c := New(srv.URL)
	got, err := c.VisionModels(context.Background())
	if err != nil || !reflect.DeepEqual(got, []string{"gemma4:12b", "gemma4:e4b"}) {
		t.Fatalf("models %v err %v", got, err)
	}
	if v, err := c.Version(context.Background()); err != nil || v != "0.32.3" {
		t.Fatalf("version %q %v", v, err)
	}
}

func TestPreferredModel(t *testing.T) {
	if got := Preferred([]string{"gemma4:12b", "llava:7b", "gemma4:e4b"}); got != "gemma4:e4b" {
		t.Fatalf("got %q", got)
	}
	if got := Preferred([]string{"llava:7b"}); got != "llava:7b" {
		t.Fatalf("got %q", got)
	}
	if Preferred(nil) != "" {
		t.Fatal("пустой список")
	}
}

func TestRespondSendsSchemaAsFormat(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		json.Unmarshal(raw, &body)
		io.WriteString(w, `{"message":{"role":"assistant","content":"{}"},"done":true}`)
	}))
	defer srv.Close()
	schema := json.RawMessage(`{"type":"object","properties":{"rows":{"type":"array"}}}`)
	if _, err := New(srv.URL).Respond(context.Background(), llm.Request{Model: "m", Text: "x", Schema: schema}); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"type": "object", "properties": map[string]any{"rows": map[string]any{"type": "array"}}}
	if !reflect.DeepEqual(body["format"], want) {
		t.Fatalf("format %v", body["format"])
	}
	body = nil
	if _, err := New(srv.URL).Respond(context.Background(), llm.Request{Model: "m", Text: "x"}); err != nil {
		t.Fatal(err)
	}
	if _, ok := body["format"]; ok {
		t.Fatal("без схемы format не передаётся")
	}
}

// Ревью M3: время вышло посреди опроса моделей — ошибка, а не «моделей нет» (иначе «Настройки» врут «не установлена»).
func TestVisionModelsTimeoutIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/tags" {
			if _, err := w.Write([]byte(`{"models":[{"name":"gemma4:e4b"}]}`)); err != nil {
				t.Errorf("ответ: %v", err)
			}
			return
		}
		select { // /api/show отвечает дольше срока клиента
		case <-r.Context().Done():
		case <-time.After(time.Second):
		}
	}))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	models, err := New(srv.URL).VisionModels(ctx)
	if err == nil {
		t.Fatalf("истёк срок — ждали ошибку, получили модели %v", models)
	}
}
