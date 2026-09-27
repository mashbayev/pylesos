// Файл: internal/recognize/extract_json.go
package recognize

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/text/unicode/norm"

	"pylesos/internal/tables"
)

// fenceRe — ```-ограждение вокруг JSON в ответе модели.
func fenceRe() *regexp.Regexp { return regexp.MustCompile("(?s)```(?:json)?\\s*(.*?)```") }

// jsonCandidates — возможные JSON-фрагменты ответа: содержимое ```-ограждения, затем каждый
// объект или массив, который удаётся разобрать (пояснения вокруг и фигурные скобки в них не мешают).
func jsonCandidates(s string) []string {
	var out []string
	if m := fenceRe().FindStringSubmatch(s); m != nil && json.Valid([]byte(strings.TrimSpace(m[1]))) {
		out = append(out, strings.TrimSpace(m[1]))
	}
	for i, r := range s {
		if r != '{' && r != '[' {
			continue
		}
		var v json.RawMessage
		if json.NewDecoder(strings.NewReader(s[i:])).Decode(&v) == nil {
			out = append(out, string(v))
		}
	}
	return out
}

func parseRows(reply string, tpl tables.Template) ([]tables.Row, error) {
	lastErr := errors.New("в ответе нет JSON")
	for _, cand := range jsonCandidates(reply) {
		rows, err := rowsFromJSON([]byte(cand), tpl)
		if err == nil {
			return rows, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

// rowsFromJSON принимает {"rows":[...]}, массив строк или одну строку-объект с колонками шаблона.
// Колонки, которых нет в ответе, получают отметку absent.
func rowsFromJSON(data []byte, tpl tables.Template) ([]tables.Row, error) {
	var items []map[string]json.RawMessage
	data = bytes.TrimSpace(data)
	if len(data) > 0 && data[0] == '[' {
		if err := json.Unmarshal(data, &items); err != nil {
			return nil, err
		}
	} else {
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(data, &obj); err != nil {
			return nil, err
		}
		if raw, ok := obj["rows"]; ok {
			if err := json.Unmarshal(raw, &items); err != nil {
				return nil, fmt.Errorf("rows: %v", err)
			}
		} else if slices.ContainsFunc(tpl.Columns, func(c tables.Column) bool { _, ok := obj[c.Name]; return ok }) {
			items = []map[string]json.RawMessage{obj}
		} else {
			return nil, errors.New("нет ни rows, ни колонок шаблона")
		}
	}
	rows := make([]tables.Row, 0, len(items))
	for _, item := range items {
		if !slices.ContainsFunc(tpl.Columns, func(c tables.Column) bool { _, ok := item[c.Name]; return ok }) {
			continue // «строка» своего устройства без колонок шаблона
		}
		row := tables.Row{Cells: make(map[string]tables.Cell, len(tpl.Columns))}
		for _, col := range tpl.Columns {
			v, ok := item[col.Name]
			if !ok {
				row.Cells[col.Name] = tables.Cell{Confidence: absent}
				continue
			}
			cell, err := cellFromJSON(v)
			if err != nil {
				return nil, fmt.Errorf("колонка %q: %v", col.Name, err)
			}
			row.Cells[col.Name] = cell
		}
		rows = append(rows, row)
	}
	if len(rows) == 0 && len(items) > 0 {
		return nil, errors.New("в строках нет колонок шаблона")
	}
	return rows, nil
}

// cellFromJSON: значение-строка, число, да/нет, null или объект {"value": …, "confidence": …}.
func cellFromJSON(v json.RawMessage) (tables.Cell, error) {
	var cell tables.Cell
	raw := bytes.TrimSpace(v)
	if len(raw) > 0 && raw[0] == '{' {
		var obj struct {
			Value      json.RawMessage `json:"value"`
			Confidence string          `json:"confidence"`
		}
		if err := json.Unmarshal(raw, &obj); err != nil {
			return cell, err
		}
		raw, cell.Confidence = obj.Value, obj.Confidence
	}
	value, err := scalar(raw)
	if err != nil {
		return cell, err
	}
	cell.Value = fixHomoglyphs(norm.NFC.String(strings.TrimSpace(value)))
	if cell.Confidence != "low" {
		cell.Confidence = "high"
	}
	return cell, nil
}

func scalar(v json.RawMessage) (string, error) {
	if len(bytes.TrimSpace(v)) == 0 {
		return "", nil
	}
	dec := json.NewDecoder(bytes.NewReader(v))
	dec.UseNumber()
	var x any
	if err := dec.Decode(&x); err != nil {
		return "", err
	}
	switch t := x.(type) {
	case nil:
		return "", nil
	case string:
		return t, nil
	case json.Number:
		return t.String(), nil
	case bool:
		return strconv.FormatBool(t), nil
	}
	return "", errors.New("значение должно быть строкой или числом")
}

func emptyRow(tpl tables.Template) tables.Row {
	cells := make(map[string]tables.Cell, len(tpl.Columns))
	for _, c := range tpl.Columns {
		cells[c.Name] = tables.Cell{Confidence: "high"}
	}
	return tables.Row{Cells: cells}
}

// mergeRows объединяет строки частей документа: первое непустое уверенное значение, иначе первое непустое.
func mergeRows(tpl tables.Template, parts [][]tables.Row) tables.Row {
	out := emptyRow(tpl)
	for _, col := range tpl.Columns {
		var fallback *tables.Cell
		for _, rows := range parts {
			for _, r := range rows {
				c := r.Cells[col.Name]
				if c.Value == "" {
					continue
				}
				if c.Confidence == "high" {
					out.Cells[col.Name] = c
					goto next
				}
				if fallback == nil {
					fallback = &c
				}
			}
		}
		if fallback != nil {
			out.Cells[col.Name] = *fallback
		}
	next:
	}
	return out
}

func rowsJSON(tpl tables.Template, rows []tables.Row) map[string]any {
	out := make([]map[string]tables.Cell, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Cells)
	}
	return map[string]any{"rows": out}
}

// rowsSchema — JSON-схема ответа извлечения: {"rows":[{"<колонка>":{"value","confidence"}}]}.
func rowsSchema(tpl tables.Template) json.RawMessage {
	cell := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"value":      map[string]any{"type": "string"},
			"confidence": map[string]any{"type": "string", "enum": []string{"high", "low"}},
		},
		"required": []string{"value", "confidence"},
	}
	props := map[string]any{}
	names := []string{}
	for _, c := range tpl.Columns {
		props[c.Name] = cell
		names = append(names, c.Name)
	}
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"rows": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "object", "properties": props, "required": names},
			},
		},
		"required": []string{"rows"},
	}
	data, err := json.Marshal(schema)
	if err != nil { // схема из строк и карт сериализуется всегда; без схемы модель ответит по инструкции промпта
		return nil
	}
	return data
}
