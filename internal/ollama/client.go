// Файл: internal/ollama/client.go
// Package ollama — распознавание локальной моделью через Ollama (http://127.0.0.1:11434).
// Реализует llm.Responder — единственная модель распознавания Пылесоса (05-recognition-llm).
package ollama

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"slices"
	"strings"
	"time"

	"pylesos/internal/llm"
)

// DefaultURL — адрес Ollama по умолчанию.
const DefaultURL = "http://127.0.0.1:11434"

var ErrNotRunning = errors.New("Ollama не запущена")

type Client struct {
	BaseURL string
	HTTP    *http.Client
}

func New(baseURL string) *Client {
	// Первая загрузка модели в память может занять пару минут, страница — до нескольких минут на CPU.
	return &Client{BaseURL: strings.TrimRight(baseURL, "/"), HTTP: &http.Client{Timeout: 15 * time.Minute}}
}

type message struct {
	Role    string   `json:"role"`
	Content string   `json:"content"`
	Images  []string `json:"images,omitempty"`
}

// Respond — один запрос к модели: системный промпт, текст и изображения.
func (c *Client) Respond(ctx context.Context, r llm.Request) (string, error) {
	user := message{Role: "user", Content: r.Text}
	for _, img := range r.Images {
		user.Images = append(user.Images, base64.StdEncoding.EncodeToString(img))
	}
	msgs := []message{}
	if r.System != "" {
		msgs = append(msgs, message{Role: "system", Content: r.System})
	}
	msgs = append(msgs, user)
	body := map[string]any{
		"model":      r.Model,
		"messages":   msgs,
		"stream":     false,
		"think":      false, // рассуждения удлиняют ответ в разы и не нужны для распознавания
		"keep_alive": "30m", // модель остаётся в памяти между страницами
		"options":    map[string]any{"temperature": 0, "num_ctx": 16384},
	}
	if len(r.Schema) > 0 {
		body["format"] = r.Schema // строгий JSON по схеме: локальная модель иначе отвечает по-своему
	}
	var resp struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		DoneReason string `json:"done_reason"`
	}
	if err := c.post(ctx, "/api/chat", body, &resp); err != nil {
		return "", err
	}
	text := strings.TrimSpace(resp.Message.Content)
	if text == "" {
		return "", fmt.Errorf("%w: пустой ответ локальной модели", llm.ErrTurnFailed)
	}
	return text, nil
}

// Version — версия Ollama (заодно проверка, что она запущена).
func (c *Client) Version(ctx context.Context) (string, error) {
	var v struct {
		Version string `json:"version"`
	}
	if err := c.get(ctx, "/api/version", &v); err != nil {
		return "", err
	}
	return v.Version, nil
}

// VisionModels — установленные модели, которые понимают изображения, по имени.
func (c *Client) VisionModels(ctx context.Context) ([]string, error) {
	var tags struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := c.get(ctx, "/api/tags", &tags); err != nil {
		return nil, err
	}
	var out []string
	for _, m := range tags.Models {
		var show struct {
			Capabilities []string `json:"capabilities"`
		}
		if err := c.post(ctx, "/api/show", map[string]string{"model": m.Name}, &show); err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err() // срок вышел — список неполный, это не «моделей нет»
			}
			continue // сведения об одной модели не отдались — считаем, что изображения она не понимает
		}
		if slices.Contains(show.Capabilities, "vision") {
			out = append(out, m.Name)
		}
	}
	slices.Sort(out)
	return out, nil
}

// Preferred — модель по умолчанию: проверенная gemma4:e4b (быстрая и точная на казахском), иначе первая.
func Preferred(models []string) string {
	for _, want := range []string{"gemma4:e4b", "gemma4-e4b-32k:latest"} {
		if slices.Contains(models, want) {
			return want
		}
	}
	if len(models) > 0 {
		return models[0]
	}
	return ""
}

func (c *Client) get(ctx context.Context, path string, dst any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+path, nil)
	if err != nil {
		return err
	}
	return c.do(req, dst)
}

func (c *Client) post(ctx context.Context, path string, body, dst any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	return c.do(req, dst)
}

func (c *Client) do(req *http.Request, dst any) (err error) {
	resp, err := c.HTTP.Do(req)
	if err != nil {
		var opErr *net.OpError
		if errors.As(err, &opErr) && opErr.Op == "dial" {
			return fmt.Errorf("%w (%v)", ErrNotRunning, err)
		}
		return err
	}
	defer func() { err = errors.Join(err, resp.Body.Close()) }()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: Ollama %d: %s", llm.ErrTurnFailed, resp.StatusCode, ollamaError(raw))
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		return fmt.Errorf("%w: %v", llm.ErrBadResponse, err)
	}
	return nil
}

// ollamaError — текст ошибки Ollama: поле error из JSON, иначе тело ответа как есть.
func ollamaError(raw []byte) string {
	var e struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(raw, &e); err != nil || e.Error == "" {
		return strings.TrimSpace(string(raw))
	}
	return e.Error
}
