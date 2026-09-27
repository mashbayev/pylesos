// Файл: internal/analyze/analyze_test.go
package analyze

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"strings"
	"sync"
	"testing"

	"pylesos/internal/llm"
)

// fakeLLM — модель по первой строке системного промпта: транскрипция, извлечение полей, поиск упоминаний.
// Все имена и ИИН вымышленные (П8).
type fakeLLM struct {
	mu       sync.Mutex
	mentions map[string]string // подстрока текста сообщения → ответ JSON
	docText  string
	docRow   string // ответ извлечения полей документа
	bad      int    // сколько раз подряд ответить мусором на поиск упоминаний
	reqs     []llm.Request
}

func (f *fakeLLM) Respond(_ context.Context, r llm.Request) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reqs = append(f.reqs, r)
	switch {
	case strings.HasPrefix(r.System, "Ты — точный распознаватель"):
		return f.docText, nil
	case strings.HasPrefix(r.System, "Ты извлекаешь данные"):
		return f.docRow, nil
	case strings.HasPrefix(r.System, "Ты разбираешь одно сообщение"):
		if f.bad > 0 {
			f.bad--
			return "не JSON", nil
		}
		own := strings.SplitN(r.Text, "\nЭто ответ на сообщение", 2)[0] // только само сообщение
		for sub, ans := range f.mentions {
			if strings.Contains(own, sub) {
				return ans, nil
			}
		}
		return `{"mentions":[]}`, nil
	}
	return "", nil
}

func mentionsJSON(t *testing.T, list ...Found) string {
	t.Helper()
	data, err := json.Marshal(map[string][]Found{"mentions": list})
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// docPNG — вымышленный «документ» для распознавания.
func docPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 200, 120))
	for y := 0; y < 120; y++ {
		for x := 0; x < 200; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 150, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
