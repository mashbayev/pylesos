// Файл: internal/instance/instance.go
// Package instance — повторный запуск программы показывает окно уже запущенной (перенос из doc-agent).
package instance

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"pylesos/internal/api"
)

type info struct {
	URL string `json:"url"`
}

// Write запоминает адрес запущенной программы (в адресе — токен, поэтому права 0600).
func Write(path, link string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("папка файла экземпляра: %w", err)
	}
	data, err := json.Marshal(info{URL: link})
	if err != nil {
		return fmt.Errorf("файл экземпляра: %w", err)
	}
	return os.WriteFile(path, data, 0o600)
}

// ShowRunning просит уже запущенную программу показать окно; false — такой нет.
func ShowRunning(ctx context.Context, client *http.Client, path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var in info
	if json.Unmarshal(data, &in) != nil {
		return false
	}
	u, err := url.Parse(in.URL)
	if err != nil || u.Query().Get("t") == "" {
		return false
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+u.Host+"/api/show", strings.NewReader("{}"))
	if err != nil {
		return false
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(api.TokenHeader, u.Query().Get("t"))
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	return resp.Body.Close() == nil && resp.StatusCode == http.StatusOK
}

// ShowRunningWithin повторяет ShowRunning, пока не выйдет время: первая копия может ещё стартовать
// (блокировка уже взята, а файл экземпляра ещё не записан).
func ShowRunningWithin(ctx context.Context, client *http.Client, path string, timeout, interval time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		if ShowRunning(ctx, client, path) {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		select {
		case <-ctx.Done():
			return false
		case <-time.After(interval):
		}
	}
}
