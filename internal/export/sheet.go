// Файл: internal/export/sheet.go
package export

import (
	"errors"
	"strings"

	"github.com/xuri/excelize/v2"

	"pylesos/internal/tables"
	"pylesos/internal/texts"
)

// sheet — запись одного листа книги.
type sheet struct {
	f    *excelize.File
	name string
	st   *styles
}

func ref(col, row int) (string, error) { return excelize.CoordinatesToCellName(col, row) }

// data — лист данных: колонки шаблона + «Файл», «Страница», «Требует проверки».
func (s *sheet) data(tpl tables.Template, rows []tables.Row) error {
	headers := make([]any, 0, len(tpl.Columns)+3)
	for _, c := range tpl.Columns {
		headers = append(headers, c.Name)
	}
	headers = append(headers, texts.XlsxFile, texts.XlsxPage, texts.XlsxReview)
	if err := s.header(headers); err != nil {
		return err
	}
	for i, r := range rows {
		if err := s.row(tpl, r, i+2); err != nil {
			return err
		}
	}
	return s.layout(len(headers))
}

func (s *sheet) row(tpl tables.Template, r tables.Row, rowNum int) error {
	var review []string
	for j, col := range tpl.Columns {
		note, err := s.cell(col, r.Cells[col.Name], j+1, rowNum)
		if err != nil {
			return err
		}
		if note != "" {
			review = append(review, col.Name+": "+note)
		}
	}
	n := len(tpl.Columns)
	values := []any{r.File, nil, nil}
	if r.Page > 0 {
		values[1] = r.Page
	}
	if len(review) > 0 {
		values[2] = strings.Join(review, "; ")
	}
	at, err := ref(n+1, rowNum)
	if err != nil {
		return err
	}
	return s.f.SetSheetRow(s.name, at, &values)
}

// cell пишет значение, стиль и примечание с замечаниями; возвращает текст замечаний ("" — нет).
func (s *sheet) cell(col tables.Column, c tables.Cell, colNum, rowNum int) (string, error) {
	at, err := ref(colNum, rowNum)
	if err != nil {
		return "", err
	}
	if err := setTyped(s.f, s.name, at, c); err != nil {
		return "", err
	}
	issues := cellIssues(c)
	if style := s.st.forType(col.Type, len(issues) > 0); style != 0 {
		if err := s.f.SetCellStyle(s.name, at, at, style); err != nil {
			return "", err
		}
	}
	if len(issues) == 0 {
		return "", nil
	}
	notes := make([]string, len(issues))
	for k, key := range issues {
		notes[k] = texts.CheckIssue(key)
	}
	comment := excelize.Comment{Author: texts.AppName, Cell: at, Paragraph: []excelize.RichTextRun{{Text: strings.Join(notes, "\n")}}}
	if err := s.f.AddComment(s.name, comment); err != nil {
		return "", err
	}
	return strings.Join(notes, ", "), nil
}

// layout — ширина колонок, закреплённая шапка и фильтр.
func (s *sheet) layout(cols int) error {
	lastCol, err := excelize.ColumnNumberToName(cols)
	if err != nil {
		return err
	}
	return errors.Join(
		s.f.SetColWidth(s.name, "A", lastCol, 22),
		s.f.SetColWidth(s.name, lastCol, lastCol, 45),
		s.f.SetPanes(s.name, &excelize.Panes{Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft"}),
		s.f.AutoFilter(s.name, "A1:"+lastCol+"1", nil),
	)
}

// report — лист отчёта: источник, страницы, статус, ошибка.
func (s *sheet) report(report []FileReport) error {
	if err := s.header([]any{texts.XlsxFile, texts.XlsxPages, texts.XlsxStatus, texts.XlsxError}); err != nil {
		return err
	}
	for i, r := range report {
		status := texts.XlsxStatusOK
		if r.Error != "" {
			status = texts.XlsxStatusError
		}
		at, err := ref(1, i+2)
		if err != nil {
			return err
		}
		if err := s.f.SetSheetRow(s.name, at, &[]any{r.File, r.Pages, status, r.Error}); err != nil {
			return err
		}
	}
	return errors.Join(s.f.SetColWidth(s.name, "A", "A", 40), s.f.SetColWidth(s.name, "D", "D", 60))
}

func (s *sheet) header(headers []any) error {
	if err := s.f.SetSheetRow(s.name, "A1", &headers); err != nil {
		return err
	}
	last, err := ref(len(headers), 1)
	if err != nil {
		return err
	}
	return s.f.SetCellStyle(s.name, "A1", last, s.st.header)
}

func setTyped(f *excelize.File, sheet, at string, c tables.Cell) error {
	switch v := c.Typed.(type) {
	case nil:
		if c.Value != "" {
			return f.SetCellStr(sheet, at, c.Value)
		}
		return nil
	case string:
		return f.SetCellStr(sheet, at, v) // текстом: ИИН с ведущим нулём не превратится в число
	default:
		return f.SetCellValue(sheet, at, v)
	}
}
