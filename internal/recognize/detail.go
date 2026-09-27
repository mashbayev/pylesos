// Файл: internal/recognize/detail.go
package recognize

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/jpeg"
	"strings"
	"unicode"

	"github.com/disintegration/imaging"

	"pylesos/internal/llm"
	"pylesos/internal/normalize"
)

// detailSide — до какой ширины увеличивается полоса; страницы, у которых короткая сторона уже
// не меньше, в подробном чтении не нуждаются (сканы, PDF).
const detailSide = 1200

// Detail — подробное чтение мелкого текста (скрины переписки, фото): изображение режется на
// перекрывающиеся квадратные полосы вдоль длинной стороны, каждая увеличивается и читается
// отдельно. Результат — дополнение к основной транскрипции для извлечения таблицы.
func (t *Transcriber) Detail(ctx context.Context, p normalize.Page) (string, error) {
	if p.Image == nil {
		return "", nil
	}
	img, _, err := image.Decode(bytes.NewReader(p.Image))
	if err != nil {
		return "", nil
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	side := min(w, h)
	if side >= detailSide {
		return "", nil
	}
	tiles := tileRects(w, h, side)
	var out strings.Builder
	prev := ""
	for i, r := range tiles {
		tile := imaging.Crop(img, r.Add(b.Min))
		if tile.Bounds().Dx() < detailSide {
			scale := float64(detailSide) / float64(tile.Bounds().Dx())
			tile = imaging.Resize(tile, detailSide, int(float64(tile.Bounds().Dy())*scale+0.5), imaging.Lanczos)
		}
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, tile, &jpeg.Options{Quality: 92}); err != nil {
			return "", err
		}
		text := fmt.Sprintf("Фрагмент %d из %d той же страницы, увеличен. Распознай текст на изображении.", i+1, len(tiles))
		reply, err := t.LLM.Respond(ctx, llm.Request{Model: t.Model, System: transcribePrompt, Text: text, Images: [][]byte{buf.Bytes()}})
		if err != nil {
			return "", err
		}
		reply = cleanTranscript(reply)
		if degenerate(reply) || reply == prev || strings.HasPrefix(reply, "Фрагмент ") {
			continue
		}
		prev = reply
		fmt.Fprintf(&out, "### Фрагмент %d\n\n%s\n\n", i+1, reply)
	}
	return strings.TrimSpace(out.String()), nil
}

// tileRects — квадраты со стороной side вдоль длинной стороны с шагом 60% (перекрытие 40%).
func tileRects(w, h, side int) []image.Rectangle {
	long, vertical := h, true
	if w > h {
		long, vertical = w, false
	}
	step := side * 6 / 10
	var starts []int
	for pos := 0; ; pos += step {
		if pos+side >= long {
			starts = append(starts, max(0, long-side))
			break
		}
		starts = append(starts, pos)
	}
	rects := make([]image.Rectangle, 0, len(starts))
	for _, s := range starts {
		if vertical {
			rects = append(rects, image.Rect(0, s, side, s+side))
		} else {
			rects = append(rects, image.Rect(s, 0, s+side, side))
		}
	}
	return rects
}

// degenerate — ответ из повторяющихся служебных символов (машиночитаемая зона «<<<<»), пустой.
func degenerate(s string) bool {
	letters, total := 0, 0
	for _, r := range s {
		if unicode.IsSpace(r) {
			continue
		}
		total++
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			letters++
		}
	}
	return total == 0 || letters*2 < total
}
