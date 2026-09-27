// Файл: internal/recognize/suggest.go
package recognize

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"

	"pylesos/internal/llm"
	"pylesos/internal/tables"
)

// Suggest предлагает шаблон по описанию и (необязательно) образцу документа.
func (e *Extractor) Suggest(ctx context.Context, description string, sample *PageText) (tables.Template, error) {
	text := "Описание пользователя: " + description
	var images [][]byte
	if sample != nil {
		text += "\n\nОбразец документа (распознанный текст):\n<<<\n" + sample.Text + "\n>>>"
		if sample.Page.Image != nil {
			images = [][]byte{sample.Page.Image}
		}
	}
	reply, err := e.ask(ctx, llm.Request{Model: e.Model, System: suggestPrompt, Text: text, Images: images})
	if err != nil {
		return tables.Template{}, err
	}
	var tpl tables.Template
	parsed := false
	for _, cand := range jsonCandidates(reply) {
		if json.Unmarshal([]byte(cand), &tpl) == nil && len(tpl.Columns) > 0 {
			parsed = true
			break
		}
	}
	if !parsed {
		return tables.Template{}, fmt.Errorf("%w: в ответе нет шаблона", ErrBadJSON)
	}
	tpl = tpl.Normalize()
	if tpl.Name == "" {
		tpl.Name = "Новый шаблон"
	}
	if err := tpl.Validate(); err != nil {
		return tables.Template{}, err
	}
	return tpl, nil
}

func templateSpec(tpl tables.Template) string {
	tpl.Forms = nil // разметка образцов — служебные данные программы, модели не нужна
	b, err := json.MarshalIndent(tpl, "", "  ")
	if err != nil { // шаблон из строк и чисел сериализуется всегда; на всякий случай — описание Go
		return fmt.Sprintf("%+v", tpl)
	}
	return string(b)
}

func pagesPayload(pages []PageText) (string, [][]byte) {
	var b strings.Builder
	var images [][]byte
	for _, p := range pages {
		fmt.Fprintf(&b, "=== Страница %d ===\n%s\n\n", p.Page.Index, p.Text)
		if p.Page.Image != nil {
			images = append(images, p.Page.Image)
		}
	}
	return strings.TrimSpace(b.String()), images
}
