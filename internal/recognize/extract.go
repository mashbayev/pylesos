// Файл: internal/recognize/extract.go
package recognize

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"pylesos/internal/llm"
	"pylesos/internal/normalize"
	"pylesos/internal/tables"
	"pylesos/internal/validate"
)

var (
	//go:embed prompts/review.md
	reviewPrompt string
	//go:embed prompts/extract.md
	extractPrompt string
	//go:embed prompts/review_rows.md
	reviewRowsPrompt string
	//go:embed prompts/suggest.md
	suggestPrompt string
)

var ErrBadJSON = errors.New("модель вернула некорректный JSON")

// logicVersion — меняется, когда запросы к модели меняются не в промптах, а в коде
// (описание шаблона, схема ответа): старые результаты из кэша тогда не используются.
const logicVersion = "2"

// PromptsVersion — отпечаток всех промптов: входит в ключ кэша, чтобы после их изменения
// документы распознавались заново.
func PromptsVersion() string {
	h := sha256.New()
	for _, p := range []string{logicVersion, transcribePrompt, reviewPrompt, extractPrompt, reviewRowsPrompt, zonePrompt} {
		h.Write([]byte(p))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil)[:8])
}

// maxImagesPerRequest — сколько страниц документа отправляется в одном запросе (режим document).
const maxImagesPerRequest = 10

// PageText — страница и её (проверенная) транскрипция.
type PageText struct {
	Page normalize.Page
	Text string
}

// Review — второй проход по транскрипции: изображение + текст → исправленный текст.
func (t *Transcriber) Review(ctx context.Context, p normalize.Page, text string) (string, error) {
	if p.Image == nil {
		return text, nil
	}
	req := fmt.Sprintf("Страница %d. Проверь транскрипцию по изображению.\n\nТранскрипция:\n<<<\n%s\n>>>", p.Index, text)
	out, err := t.LLM.Respond(ctx, llm.Request{Model: t.Model, System: reviewPrompt, Text: req, Images: [][]byte{p.Image}})
	if err != nil {
		return "", err
	}
	if out = cleanTranscript(out); out == "" {
		return "", ErrEmpty
	}
	// Локальные модели иногда возвращают текст самого запроса вместо страницы — такой ответ
	// не исправление, остаётся первый проход.
	if strings.HasPrefix(out, fmt.Sprintf("Страница %d. Проверь транскрипцию", p.Index)) {
		return text, nil
	}
	return out, nil
}

// Extractor извлекает строки таблицы по шаблону.
type Extractor struct {
	LLM   llm.Responder
	Model string
	Pause time.Duration // пауза между собственными запросами
	// TextOnly — собирать таблицу по уже проверенному тексту, не загружая изображения снова:
	// на бесплатном тарифе ChatGPT загрузки картинок ограничены.
	TextOnly bool
	sent     bool
}

// Extract возвращает строки для одного файла. review — второй проход по строкам:
// изменённые при проверке ячейки получают низкую уверенность.
func (e *Extractor) Extract(ctx context.Context, tpl tables.Template, source string, pages []PageText, review bool) ([]tables.Row, error) {
	tpl = tpl.Normalize()
	if len(tpl.Forms) == 0 || tpl.Mode == tables.ModeTableRows {
		return e.extract(ctx, tpl, source, pages, review)
	}
	if tpl.Mode == tables.ModePeople {
		// Сначала документы по образцам, затем переписка вокруг — общее извлечение с их значениями.
		withDocs, err := e.withFoundDocuments(ctx, tpl, pages)
		if err != nil {
			return nil, err
		}
		return e.extract(ctx, tpl, source, withDocs, review)
	}
	// Зоны на образце: общее извлечение — только если зоны покрывают не все колонки.
	var rows []tables.Row
	if zonesCoverAll(tpl, pages) {
		if tpl.Mode == tables.ModeDocument {
			row := emptyRow(tpl)
			row.File = source
			rows = []tables.Row{row}
		} else {
			for _, p := range pages {
				row := emptyRow(tpl)
				row.File, row.Page = source, p.Page.Index
				rows = append(rows, row)
			}
		}
	} else {
		var err error
		if rows, err = e.extract(ctx, tpl, source, pages, review); err != nil {
			return nil, err
		}
	}
	rowFor := func(page int) *tables.Row {
		for i := range rows {
			if tpl.Mode == tables.ModeDocument || rows[i].Page == page {
				return &rows[i]
			}
		}
		return &rows[0]
	}
	if err := e.applyZones(ctx, tpl, pages, rowFor); err != nil {
		return nil, err
	}
	return rows, nil
}

func (e *Extractor) extract(ctx context.Context, tpl tables.Template, source string, pages []PageText, review bool) ([]tables.Row, error) {
	switch tpl.Mode {
	case tables.ModeDocument:
		var parts [][]tables.Row
		for start := 0; start < len(pages); start += maxImagesPerRequest {
			rows, err := e.unit(ctx, tpl, pages[start:min(start+maxImagesPerRequest, len(pages))], review)
			if err != nil {
				return nil, err
			}
			parts = append(parts, rows)
		}
		row := mergeRows(tpl, parts)
		row.File = source
		return []tables.Row{row}, nil
	default:
		var out []tables.Row
		for _, p := range pages {
			rows, err := e.unit(ctx, tpl, []PageText{p}, review)
			if err != nil {
				return nil, err
			}
			if tpl.Mode == tables.ModePage {
				if len(rows) == 0 {
					rows = []tables.Row{emptyRow(tpl)}
				}
				rows = rows[:1]
			}
			for _, r := range rows {
				r.File, r.Page = source, p.Page.Index
				out = append(out, r)
			}
		}
		return out, nil
	}
}

// unit — один запрос извлечения (с повтором при битом JSON) и, при review, один запрос проверки.
func (e *Extractor) unit(ctx context.Context, tpl tables.Template, pages []PageText, review bool) ([]tables.Row, error) {
	spec := templateSpec(tpl)
	text, images := pagesPayload(pages)
	if e.TextOnly {
		images = nil
	}
	rows, err := e.askRows(ctx, extractPrompt+"\n\nШаблон:\n"+spec, text, images, tpl)
	if err != nil {
		return nil, err
	}
	if !review {
		return finalize(rows), nil
	}
	current, err := json.Marshal(rowsJSON(tpl, finalize(cloneRows(rows))))
	if err != nil {
		return nil, fmt.Errorf("строки для самопроверки: %w", err)
	}
	checked, err := e.askRows(ctx, reviewRowsPrompt+"\n\nШаблон:\n"+spec, text+"\n\nИзвлечённые строки:\n"+string(current), images, tpl)
	if errors.Is(err, ErrBadJSON) {
		return finalize(rows), nil // проверка — дополнительный шаг: без неё остаются строки первого прохода
	}
	if err != nil {
		return nil, err
	}
	return finalize(applyReview(tpl, finalize(rows), checked)), nil
}

// applyReview переносит правки второго прохода: изменённое значение получает низкую уверенность,
// пропущенное проверяющим — остаётся из первого прохода. Если проверяющий изменил число строк,
// остаются строки первого прохода, все помеченные как сомнительные: так ничего не теряется молча.
func applyReview(tpl tables.Template, orig, checked []tables.Row) []tables.Row {
	if len(checked) == 0 {
		return orig
	}
	out := make([]tables.Row, len(orig))
	for i, r := range orig {
		cells := make(map[string]tables.Cell, len(r.Cells))
		for _, col := range tpl.Columns {
			oc := r.Cells[col.Name]
			if len(checked) != len(orig) {
				oc.Confidence = "low"
			} else if nc := checked[i].Cells[col.Name]; nc.Confidence != absent {
				if !sameValue(col, nc.Value, oc.Value) {
					oc.Value, oc.Confidence = nc.Value, "low"
				} else if nc.Confidence == "low" {
					oc.Confidence = "low"
				}
			}
			cells[col.Name] = oc
		}
		out[i] = tables.Row{File: r.File, Page: r.Page, Cells: cells}
	}
	return out
}

// sameValue — одно и то же значение в разной записи («2019 жылғы 1 қыркүйек» и «01.09.2019»,
// «В» кириллицей и «B» латиницей в списке): правкой не считается.
func sameValue(col tables.Column, a, b string) bool {
	if a == b {
		return true
	}
	ra, rb := validate.Check(col, a), validate.Check(col, b)
	return len(ra.Issues) == 0 && len(rb.Issues) == 0 && ra.Value == rb.Value
}

// absent — служебная отметка «модель не вернула эту колонку»; до выхода из пакета становится "high".
const absent = "absent"

func finalize(rows []tables.Row) []tables.Row {
	for _, r := range rows {
		for name, c := range r.Cells {
			if c.Confidence == absent {
				c.Confidence = "high"
				r.Cells[name] = c
			}
		}
	}
	return rows
}

func cloneRows(rows []tables.Row) []tables.Row {
	out := make([]tables.Row, len(rows))
	for i, r := range rows {
		cells := make(map[string]tables.Cell, len(r.Cells))
		for k, v := range r.Cells {
			cells[k] = v
		}
		out[i] = tables.Row{File: r.File, Page: r.Page, Cells: cells}
	}
	return out
}

func (e *Extractor) askRows(ctx context.Context, system, text string, images [][]byte, tpl tables.Template) ([]tables.Row, error) {
	schema := rowsSchema(tpl)
	reply, err := e.ask(ctx, llm.Request{Model: e.Model, System: system, Text: text, Images: images, Schema: schema})
	if err != nil {
		return nil, err
	}
	rows, perr := parseRows(reply, tpl)
	if perr == nil {
		return rows, nil
	}
	retry := text + fmt.Sprintf("\n\nПредыдущий ответ не был корректным JSON (%v). Верни только JSON в указанном формате.", perr)
	if reply, err = e.ask(ctx, llm.Request{Model: e.Model, System: system, Text: retry, Images: images, Schema: schema}); err != nil {
		return nil, err
	}
	if rows, perr = parseRows(reply, tpl); perr != nil {
		return nil, fmt.Errorf("%w: %v", ErrBadJSON, perr)
	}
	return rows, nil
}

func (e *Extractor) ask(ctx context.Context, r llm.Request) (string, error) {
	if e.sent && e.Pause > 0 {
		timer := time.NewTimer(e.Pause)
		select {
		case <-ctx.Done():
			timer.Stop()
			return "", ctx.Err()
		case <-timer.C:
		}
	}
	e.sent = true
	return e.LLM.Respond(ctx, r)
}
