// Файл: internal/llm/llm.go
// Package llm — запрос к модели распознавания (только локальная Ollama, 05-recognition-llm).
// Замена core.Request из doc-agent (П5): ChatGPT Web в Пылесос не переносится.
package llm

import (
	"context"
	"encoding/json"
	"errors"
)

// Request — один ход: системный промпт, текст и изображения (JPEG/PNG).
type Request struct {
	Model  string
	System string
	Text   string
	Images [][]byte
	// Schema — JSON-схема ожидаемого ответа; Ollama по ней ограничивает вывод модели.
	Schema json.RawMessage
}

// Responder — модель, отвечающая на запрос текстом.
type Responder interface {
	Respond(ctx context.Context, r Request) (string, error)
}

var (
	ErrBadResponse = errors.New("некорректный ответ модели")
	ErrTurnFailed  = errors.New("модель не ответила")
)
