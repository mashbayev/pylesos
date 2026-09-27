// Файл: internal/recognize/zones_test.go
package recognize

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"strings"
	"testing"

	"pylesos/internal/normalize"
	"pylesos/internal/tables"
)

// layout рисует страницу 600×800: белый фон и тёмный блок в заданном месте (раскладка документа).
func layout(t *testing.T, x0, y0, x1, y1 int, shift int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 600, 800))
	for y := 0; y < 800; y++ {
		for x := 0; x < 600; x++ {
			c := color.RGBA{255, 255, 255, 255}
			if x >= x0+shift && x < x1+shift && y >= y0+shift && y < y1+shift {
				c = color.RGBA{20, 20, 20, 255}
			}
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestSignatureMatchesSameLayout(t *testing.T) {
	idCard := layout(t, 50, 50, 300, 200, 0)
	license := layout(t, 300, 500, 550, 750, 0)
	sigID, err := Signature(idCard)
	if err != nil {
		t.Fatal(err)
	}
	sigLic, _ := Signature(license)
	forms := []tables.Form{{Name: "Удостоверение", Signature: sigID}, {Name: "Права", Signature: sigLic}}

	page := layout(t, 50, 50, 300, 200, 6) // тот же вид, чуть сдвинут
	f, score := MatchForm(forms, page)
	if f == nil || f.Name != "Удостоверение" || score < MatchThreshold {
		t.Fatalf("вид %v, сходство %.2f", f, score)
	}
	f, _ = MatchForm(forms, layout(t, 300, 500, 550, 750, 0))
	if f == nil || f.Name != "Права" {
		t.Fatalf("права: %v", f)
	}
	if f, score := MatchForm(forms, layout(t, 0, 380, 600, 420, 0)); f != nil {
		t.Fatalf("непохожая страница не должна сопоставляться: %v %.2f", f.Name, score)
	}
}

func TestCropZoneAddsMarginAndUpscales(t *testing.T) {
	page := layout(t, 0, 0, 1, 1, 0) // 600×800
	crop, err := cropZone(page, tables.Zone{X: 0.1, Y: 0.1, W: 0.2, H: 0.05})
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(crop))
	if err != nil {
		t.Fatal(err)
	}
	// Зона 120×40 px; запас 10% ширины (не меньше 12 px) и 25% высоты (не меньше 12 px) → 144×64,
	// затем увеличение до ширины 800 → 800×355.
	if cfg.Width != 800 || cfg.Height < 350 || cfg.Height > 360 {
		t.Fatalf("размер вырезки %dx%d", cfg.Width, cfg.Height)
	}
}

func zonePages(t *testing.T) ([]PageText, string) {
	idCard := layout(t, 50, 50, 300, 200, 0)
	sig, _ := Signature(idCard)
	return []PageText{{Page: normalize.Page{Source: "a.pdf", Index: 1, Image: idCard}, Text: "общий текст"}}, sig
}

func TestExtractWithZonesCoveringAllColumns(t *testing.T) {
	pages, sig := zonePages(t)
	tpl := driverTpl
	tpl.Forms = []tables.Form{{Name: "Удостоверение", Signature: sig, Zones: map[string]tables.Zone{
		"ФИО": {X: 0.1, Y: 0.1, W: 0.4, H: 0.05},
		"ИИН": {X: 0.1, Y: 0.2, W: 0.4, H: 0.05},
	}}}
	q := &queueCore{replies: []string{`{"value":"ӘБДІХАЛЫҚОВ","confidence":"high"}`, `{"value":"850315400124"}`}}
	rows, err := (&Extractor{LLM: q, Model: "m", TextOnly: true}).Extract(context.Background(), tpl, "a.pdf", pages, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(q.reqs) != 2 {
		t.Fatalf("при полном покрытии зонами общее извлечение не нужно: запросов %d", len(q.reqs))
	}
	for i, col := range []string{"ФИО", "ИИН"} {
		r := q.reqs[i]
		if r.System != zonePrompt || len(r.Images) != 1 || !strings.Contains(r.Text, "«"+col+"»") {
			t.Fatalf("запрос %d: %q, картинок %d", i, r.Text, len(r.Images))
		}
	}
	c := rows[0].Cells
	if c["ФИО"].Value != "ӘБДІХАЛЫҚОВ" || c["ИИН"].Value != "850315400124" || rows[0].File != "a.pdf" {
		t.Fatalf("row %+v", rows[0])
	}
}

func TestExtractZonesOverrideGeneralExtraction(t *testing.T) {
	pages, sig := zonePages(t)
	tpl := driverTpl
	tpl.Forms = []tables.Form{{Name: "Удостоверение", Signature: sig, Zones: map[string]tables.Zone{
		"ФИО": {X: 0.1, Y: 0.1, W: 0.4, H: 0.05},
	}}}
	q := &queueCore{replies: []string{
		`{"rows":[{"ФИО":"Тегі / Фамилия: ӘБДІХАЛЫҚОВ","ИИН":"850315400124"}]}`,
		"ӘБДІХАЛЫҚОВ", // модель ответила просто текстом — принимается как значение с низкой уверенностью
	}}
	rows, err := (&Extractor{LLM: q, Model: "m", TextOnly: true}).Extract(context.Background(), tpl, "a.pdf", pages, false)
	if err != nil {
		t.Fatal(err)
	}
	c := rows[0].Cells
	if c["ФИО"].Value != "ӘБДІХАЛЫҚОВ" || c["ФИО"].Confidence != "low" || c["ИИН"].Value != "850315400124" {
		t.Fatalf("cells %+v", c)
	}
}

func TestExtractIgnoresZonesForUnmatchedPages(t *testing.T) {
	pages, _ := zonePages(t)
	other, _ := Signature(layout(t, 300, 500, 550, 750, 0))
	tpl := driverTpl
	tpl.Forms = []tables.Form{{Name: "Права", Signature: other, Zones: map[string]tables.Zone{"ФИО": {X: 0.1, Y: 0.1, W: 0.4, H: 0.05}}}}
	q := &queueCore{replies: []string{`{"rows":[{"ФИО":"А","ИИН":"1"}]}`}}
	if _, err := (&Extractor{LLM: q, Model: "m", TextOnly: true}).Extract(context.Background(), tpl, "a.pdf", pages, false); err != nil {
		t.Fatal(err)
	}
	if len(q.reqs) != 1 {
		t.Fatalf("фото/скан другого вида — только общее извлечение: запросов %d", len(q.reqs))
	}
}
