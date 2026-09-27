// Файл: internal/analyze/mentions.go
// Package analyze — разбор Telegram (спецификация, раздел 4): документы, упоминания людей, привязка.
package analyze

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	docdomain "pylesos/internal/docforms/domain"
	"pylesos/internal/llm"
	tgdomain "pylesos/internal/telegram/domain"
)

//go:embed prompts/mentions.md
var mentionsPrompt string

var ErrBadAnswer = errors.New("модель вернула некорректный ответ")

// Found — упомянутый в сообщении человек (ответ модели).
type Found struct {
	Name string `json:"name"`
	IIN  string `json:"iin"`
	Role string `json:"role"`
}

// Finder находит упомянутых людей и их роль одним запросом к модели по JSON-схеме.
type Finder struct{ LLM llm.Responder }

// Find — люди сообщения; docs — поля его документов, reply — текст сообщения, на которое это ответ.
// Некорректный ответ — один повтор, затем ErrBadAnswer (05-recognition-llm).
func (f *Finder) Find(ctx context.Context, model string, msg tgdomain.Message, docs []map[string]string, reply string) ([]Found, error) {
	req := llm.Request{Model: model, System: mentionsPrompt, Text: mentionInput(msg, docs, reply), Schema: mentionsSchema()}
	for attempt := 0; attempt < 2; attempt++ {
		out, err := f.LLM.Respond(ctx, req)
		if err != nil {
			return nil, err
		}
		if found, ok := parseFound(out); ok {
			return found, nil
		}
	}
	return nil, ErrBadAnswer
}

func mentionInput(msg tgdomain.Message, docs []map[string]string, reply string) string {
	var b strings.Builder
	b.WriteString("Сообщение:\n" + strings.TrimSpace(msg.Text) + "\n")
	for i, d := range docs {
		fmt.Fprintf(&b, "\nДокумент %d в сообщении:\n", i+1)
		for _, key := range docdomain.Fields() {
			if v := d[key]; v != "" {
				fmt.Fprintf(&b, "- %s: %s\n", docdomain.FieldLabel(key), v)
			}
		}
	}
	if strings.TrimSpace(reply) != "" {
		b.WriteString("\nЭто ответ на сообщение:\n" + strings.TrimSpace(reply) + "\n")
	}
	return b.String()
}

// parseFound — упоминания из ответа; записи с неизвестной ролью отбрасываются.
func parseFound(out string) ([]Found, bool) {
	var ans struct {
		Mentions []Found `json:"mentions"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &ans); err != nil || ans.Mentions == nil {
		return nil, false
	}
	found := []Found{}
	for _, m := range ans.Mentions {
		switch m.Role {
		case "record", "query", "comment":
			found = append(found, m)
		}
	}
	return found, true
}

func mentionsSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"mentions":{"type":"array","items":{"type":"object",` +
		`"properties":{"name":{"type":"string"},"iin":{"type":"string"},"role":{"type":"string","enum":["record","query","comment"]}},` +
		`"required":["name","iin","role"]}}},"required":["mentions"]}`)
}
