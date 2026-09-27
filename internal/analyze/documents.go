// Файл: internal/analyze/documents.go
package analyze

import (
	"context"
	"errors"
	"strings"

	docdomain "pylesos/internal/docforms/domain"
	"pylesos/internal/llm"
	"pylesos/internal/recognize"
	"pylesos/internal/tables"
	tgdomain "pylesos/internal/telegram/domain"
)

// errUnreadable — файл не читается как документ (формат, пароль, повреждение): распознавать нечего.
var errUnreadable = errors.New("вложение не читается как документ")

// DocReader — текст документа и его опознавательные поля (спецификация, раздел 4, шаг 1).
type DocReader struct {
	LLM   llm.Responder
	Pages Pages
	Forms Forms
}

// Read: страницы → текст каждой (Transcriber) → поля по формам документов (зоны) или моделью по всему тексту.
func (d *DocReader) Read(ctx context.Context, model string, f tgdomain.File) (string, map[string]string, error) {
	pages, err := d.Pages.Bytes(f.Name, f.Data)
	if err != nil {
		return "", nil, errors.Join(errUnreadable, err)
	}
	tr := &recognize.Transcriber{LLM: d.LLM, Model: model}
	texts := make([]recognize.PageText, 0, len(pages))
	for _, p := range pages {
		text, err := tr.Page(ctx, p)
		if err != nil {
			return "", nil, err
		}
		texts = append(texts, recognize.PageText{Page: p, Text: text})
	}
	tpl, err := d.template(ctx)
	if err != nil {
		return "", nil, err
	}
	rows, err := (&recognize.Extractor{LLM: d.LLM, Model: model}).Extract(ctx, tpl, f.Name, texts, false)
	if err != nil {
		return "", nil, err
	}
	return joinTexts(texts), fieldsOf(rows), nil
}

// template — шаблон из четырёх опознавательных полей и размеченных форм документов (П21).
func (d *DocReader) template(ctx context.Context) (tables.Template, error) {
	tpl := tables.Template{Name: "Документ", Mode: tables.ModeDocument}
	for _, key := range docdomain.Fields() {
		if col, ok := docdomain.FieldColumn(key); ok {
			tpl.Columns = append(tpl.Columns, col)
		}
	}
	forms, err := d.Forms.List(ctx)
	if err != nil {
		return tables.Template{}, err
	}
	for _, f := range forms {
		if len(f.Zones) > 0 {
			tpl.Forms = append(tpl.Forms, f.AsTablesForm())
		}
	}
	return tpl, nil
}

func fieldsOf(rows []tables.Row) map[string]string {
	fields := map[string]string{}
	if len(rows) == 0 {
		return fields
	}
	for _, key := range docdomain.Fields() {
		col, ok := docdomain.FieldColumn(key)
		if !ok {
			continue
		}
		if v := strings.TrimSpace(rows[0].Cells[col.Name].Value); v != "" {
			fields[key] = v
		}
	}
	return fields
}

func joinTexts(texts []recognize.PageText) string {
	parts := make([]string, 0, len(texts))
	for _, t := range texts {
		if s := strings.TrimSpace(t.Text); s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, "\n\n")
}
