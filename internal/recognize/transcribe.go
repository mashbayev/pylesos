// Файл: internal/recognize/transcribe.go
// Package recognize распознаёт страницы через ядро.
package recognize

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/text/unicode/norm"

	"pylesos/internal/llm"
	"pylesos/internal/normalize"
)

//go:embed prompts/transcribe.md
var transcribePrompt string

var ErrEmpty = errors.New("модель вернула пустую транскрипцию")

type Transcriber struct {
	LLM   llm.Responder
	Model string
}

// Page возвращает дословный текст страницы. Страницы без изображения не отправляются в модель.
// Имя файла модели не передаётся — только номер страницы.
func (t *Transcriber) Page(ctx context.Context, p normalize.Page) (string, error) {
	if p.Image == nil {
		return norm.NFC.String(strings.TrimSpace(p.TextLayer)), nil
	}
	text := fmt.Sprintf("Страница %d. Распознай текст на изображении.", p.Index)
	if p.TextLayer != "" {
		text += "\n\nТекстовый слой (подсказка; источник истины — изображение):\n<<<\n" + p.TextLayer + "\n>>>"
	}
	out, err := t.LLM.Respond(ctx, llm.Request{Model: t.Model, System: transcribePrompt, Text: text, Images: [][]byte{p.Image}})
	if err != nil {
		return "", err
	}
	out = cleanTranscript(out)
	if out == "" {
		return "", ErrEmpty
	}
	return out, nil
}

// cleanTranscript снимает внешнее ```-ограждение, приводит текст к NFC и чинит похожие буквы.
func cleanTranscript(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		if i := strings.IndexByte(s, '\n'); i >= 0 {
			s = s[i+1:]
		} else {
			s = ""
		}
		s = strings.TrimSuffix(strings.TrimSpace(s), "```")
	}
	return fixHomoglyphs(norm.NFC.String(strings.TrimSpace(s)))
}
