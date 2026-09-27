// Файл: internal/recognize/zones.go
package recognize

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"math"
	"strings"

	"github.com/disintegration/imaging"

	"pylesos/internal/llm"
	"pylesos/internal/tables"
)

//go:embed prompts/zone.md
var zonePrompt string

// MatchThreshold — минимальное сходство страницы с образцом, чтобы применить его зоны.
// Замер на выгрузках Kaspi: один вид 0,86–0,99, разные виды 0,67–0,75.
const MatchThreshold = 0.80

const sigSide = 32

// Signature — отпечаток образца: миниатюра 32×32 в оттенках серого, base64 PNG.
func Signature(jpg []byte) (string, error) {
	img, _, err := image.Decode(bytes.NewReader(jpg))
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, imaging.Resize(imaging.Grayscale(img), sigSide, sigSide, imaging.Box)); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

// vector — нормированные яркости миниатюры (среднее 0, отклонение 1) для корреляции.
func vector(img image.Image) []float64 {
	small := imaging.Resize(imaging.Grayscale(img), sigSide, sigSide, imaging.Box)
	v := make([]float64, 0, sigSide*sigSide)
	var mean float64
	for y := 0; y < sigSide; y++ {
		for x := 0; x < sigSide; x++ {
			f := float64(small.NRGBAAt(x, y).R)
			v = append(v, f)
			mean += f
		}
	}
	mean /= float64(len(v))
	var sd float64
	for i := range v {
		v[i] -= mean
		sd += v[i] * v[i]
	}
	sd = math.Sqrt(sd/float64(len(v))) + 1e-9
	for i := range v {
		v[i] /= sd
	}
	return v
}

func correlation(a, b []float64) float64 {
	var s float64
	for i := range a {
		s += a[i] * b[i]
	}
	return s / float64(len(a))
}

// MatchForm — вид документа, на образец которого страница похожа больше всего (или nil).
// Сопоставляются только виды на всю страницу (без рамки): карточки ищутся Locate.
func MatchForm(forms []tables.Form, pageJPEG []byte) (*tables.Form, float64) {
	page, _, err := image.Decode(bytes.NewReader(pageJPEG))
	if err != nil {
		return nil, 0
	}
	pv := vector(page)
	var best *tables.Form
	bestScore := -1.0
	for i := range forms {
		if forms[i].Frame != nil {
			continue
		}
		raw, err := base64.StdEncoding.DecodeString(forms[i].Signature)
		if err != nil {
			continue
		}
		sig, err := png.Decode(bytes.NewReader(raw))
		if err != nil {
			continue
		}
		if s := correlation(pv, vector(sig)); s > bestScore {
			best, bestScore = &forms[i], s
		}
	}
	if bestScore < MatchThreshold {
		return nil, bestScore
	}
	return best, bestScore
}

// cropZone вырезает зону с запасом на сдвиг скана и увеличивает мелкий текст.
func cropZone(pageJPEG []byte, z tables.Zone) ([]byte, error) {
	page, _, err := image.Decode(bytes.NewReader(pageJPEG))
	if err != nil {
		return nil, err
	}
	b := page.Bounds()
	w, h := float64(b.Dx()), float64(b.Dy())
	zw, zh := z.W*w, z.H*h
	padX, padY := math.Max(12, 0.10*zw), math.Max(12, 0.25*zh)
	rect := image.Rect(
		int(math.Max(0, z.X*w-padX)), int(math.Max(0, z.Y*h-padY)),
		int(math.Min(w, z.X*w+zw+padX)), int(math.Min(h, z.Y*h+zh+padY)),
	).Add(b.Min)
	img := imaging.Crop(page, rect)
	if img.Bounds().Dx() < 800 {
		img = imaging.Resize(img, 800, 0, imaging.Lanczos)
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 92}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// readZone — значение одной колонки по вырезанной зоне.
func (e *Extractor) readZone(ctx context.Context, col tables.Column, crop []byte) (tables.Cell, error) {
	text := fmt.Sprintf("Поле: «%s».", col.Name)
	if col.Hint != "" {
		text += " Подсказка: " + col.Hint + "."
	}
	if len(col.Options) > 0 {
		text += " Допустимые значения: " + strings.Join(col.Options, ", ") + "."
	}
	reply, err := e.ask(ctx, llm.Request{Model: e.Model, System: zonePrompt, Text: text, Images: [][]byte{crop}})
	if err != nil {
		return tables.Cell{}, err
	}
	for _, cand := range jsonCandidates(reply) {
		var obj map[string]json.RawMessage
		if json.Unmarshal([]byte(cand), &obj) == nil {
			if _, ok := obj["value"]; ok {
				return cellFromJSON(json.RawMessage(cand))
			}
		}
	}
	// Модель ответила просто текстом: берём его, но помечаем для проверки.
	value := cleanTranscript(reply)
	if strings.Contains(value, "Поле: «") {
		value = "" // эхо запроса
	}
	return tables.Cell{Value: value, Confidence: "low"}, nil
}

// applyZones читает зоны страниц, похожих на образцы, и заменяет ими значения строк.
// rowFor возвращает строку, в которую пишутся значения страницы.
func (e *Extractor) applyZones(ctx context.Context, tpl tables.Template, pages []PageText, rowFor func(page int) *tables.Row) error {
	done := map[string]bool{}
	for _, p := range pages {
		if p.Page.Image == nil {
			continue
		}
		docs, err := e.findDocuments(ctx, tpl, p.Page.Image)
		if err != nil {
			return err
		}
		row := rowFor(p.Page.Index)
		for _, d := range docs {
			for name, cell := range d.cells {
				key := fmt.Sprintf("%d:%s", pageKey(tpl, p.Page.Index), name)
				if !done[key] {
					row.Cells[name] = cell
					done[key] = true
				}
			}
		}
	}
	return nil
}

// pageKey: в режиме документа все страницы пишут в одну строку, в режиме страницы — каждая в свою.
func pageKey(tpl tables.Template, page int) int {
	if tpl.Mode == tables.ModeDocument {
		return 0
	}
	return page
}

// zonesCoverAll — все ли колонки получат значение из зон (тогда общее извлечение не нужно).
func zonesCoverAll(tpl tables.Template, pages []PageText) bool {
	covered := map[string]bool{}
	for _, p := range pages {
		if p.Page.Image == nil {
			continue
		}
		if form, _ := MatchForm(tpl.Forms, p.Page.Image); form != nil {
			for name := range form.Zones {
				covered[name] = true
			}
		}
	}
	for _, c := range tpl.Columns {
		if !covered[c.Name] {
			return false
		}
	}
	return true
}

// foundDocument — документ, найденный на странице по образцу, и прочитанные по зонам значения.
type foundDocument struct {
	form  string
	cells map[string]tables.Cell
}

// readForm читает зоны вида документа; match — найденная карточка (nil — вид на всю страницу).
// Нечитаемая страница или зона — не ошибка разбора, а «зон нет»: такие ячейки прочитает модель
// по всей странице (запасной путь doc-agent, ревью M3).
func (e *Extractor) readForm(ctx context.Context, tpl tables.Template, form tables.Form, pageJPEG []byte, match *Match) (map[string]tables.Cell, error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(pageJPEG))
	if err != nil {
		return nil, nil // страница не декодируется — зоны не читаем, работает общий путь
	}
	cells := map[string]tables.Cell{}
	for _, col := range tpl.Columns {
		z, ok := form.Zones[col.Name]
		if !ok {
			continue
		}
		if match != nil {
			z = zoneInMatch(z, *form.Frame, match.Rect, cfg.Width, cfg.Height)
			if z.X < 0 || z.Y < 0 || z.X+z.W > 1 || z.Y+z.H > 1 {
				continue // зона найденной карточки выходит за край скрина (карточка обрезана)
			}
		}
		crop, err := cropZone(pageJPEG, z)
		if err != nil {
			continue // зону не вырезать — это поле прочитает модель по всей странице
		}
		cell, err := e.readZone(ctx, col, crop)
		if err != nil {
			return nil, err
		}
		if cell.Value != "" {
			cells[col.Name] = cell
		}
	}
	return cells, nil
}

// findDocuments — документы страницы: вид на всю страницу (выгрузки Kaspi) или карточки внутри
// скрина переписки, найденные по образцу; для каждого — значения его зон.
func (e *Extractor) findDocuments(ctx context.Context, tpl tables.Template, pageJPEG []byte) ([]foundDocument, error) {
	var out []foundDocument
	if form, _ := MatchForm(tpl.Forms, pageJPEG); form != nil {
		cells, err := e.readForm(ctx, tpl, *form, pageJPEG, nil)
		if err != nil {
			return nil, err
		}
		if len(cells) > 0 {
			out = append(out, foundDocument{form.Name, cells})
		}
		return out, nil
	}
	for _, form := range tpl.Forms {
		if form.Frame == nil || form.Locator == "" {
			continue
		}
		for _, m := range Locate(form.Locator, pageJPEG) {
			cells, err := e.readForm(ctx, tpl, form, pageJPEG, &m)
			if err != nil {
				return nil, err
			}
			if len(cells) > 0 {
				out = append(out, foundDocument{form.Name, cells})
			}
		}
	}
	return out, nil
}

// withFoundDocuments добавляет к тексту страниц значения документов, прочитанные по зонам:
// сначала документ, потом — всё, что вокруг него (переписка), разбирает общее извлечение.
func (e *Extractor) withFoundDocuments(ctx context.Context, tpl tables.Template, pages []PageText) ([]PageText, error) {
	out := make([]PageText, len(pages))
	copy(out, pages)
	for i, p := range out {
		if p.Page.Image == nil {
			continue
		}
		docs, err := e.findDocuments(ctx, tpl, p.Page.Image)
		if err != nil {
			return nil, err
		}
		if len(docs) == 0 {
			continue
		}
		var b strings.Builder
		b.WriteString("## Документы, найденные по образцам (значения прочитаны прямо с документа — точные)\n\n")
		for n, d := range docs {
			fmt.Fprintf(&b, "%d) %s:", n+1, d.form)
			for _, col := range tpl.Columns {
				if c, ok := d.cells[col.Name]; ok {
					fmt.Fprintf(&b, " %s: %s;", col.Name, c.Value)
				}
			}
			b.WriteString("\n")
		}
		out[i].Text = b.String() + "\n" + p.Text
	}
	return out, nil
}
