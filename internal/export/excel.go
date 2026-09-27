// Файл: internal/export/excel.go
// Package export пишет таблицу Excel по шаблону: лист данных и лист отчёта (перенос из doc-agent, П5).
package export

import (
	"errors"
	"strings"

	"github.com/xuri/excelize/v2"

	"pylesos/internal/tables"
	"pylesos/internal/texts"
)

// FileReport — строка листа «Отчёт»: источник, число страниц и ошибка (готовым текстом).
type FileReport struct {
	File  string
	Pages int
	Error string
}

// ErrExcelLocked — не удалось записать таблицу ни под одним именем (файлы открыты в Excel).
var ErrExcelLocked = errors.New(texts.ErrXlsxLocked)

// ExcelName — имя .xlsx для шаблона: запрещённые в именах файлов символы заменяются на «_».
func ExcelName(tpl tables.Template) string {
	var b strings.Builder
	for _, r := range tpl.Name {
		if r < 32 || strings.ContainsRune(`/\:*?"<>|`, r) {
			r = '_'
		}
		b.WriteRune(r)
	}
	s := strings.Trim(b.String(), " .")
	if r := []rune(s); len(r) > 100 {
		s = string(r[:100])
	}
	if s == "" {
		s = "таблица"
	}
	return s + ".xlsx"
}

// cellIssues — ключи замечаний ячейки, включая низкую уверенность модели.
func cellIssues(c tables.Cell) []string {
	issues := append([]string(nil), c.Issues...)
	if c.Confidence == "low" {
		issues = append(issues, "chk.low_confidence")
	}
	return issues
}

// WriteExcel пишет таблицу (лист данных + лист отчёта) в outDir и возвращает путь к файлу.
func WriteExcel(outDir string, tpl tables.Template, rows []tables.Row, report []FileReport) (path string, err error) {
	f := excelize.NewFile()
	defer func() { err = errors.Join(err, f.Close()) }()
	if err := f.SetSheetName("Sheet1", texts.XlsxSheetData); err != nil {
		return "", err
	}
	if _, err := f.NewSheet(texts.XlsxSheetReport); err != nil {
		return "", err
	}
	st, err := newStyles(f)
	if err != nil {
		return "", err
	}
	w := &sheet{f: f, name: texts.XlsxSheetData, st: st}
	if err := w.data(tpl, rows); err != nil {
		return "", err
	}
	if err := (&sheet{f: f, name: texts.XlsxSheetReport, st: st}).report(report); err != nil {
		return "", err
	}
	return save(f, outDir, ExcelName(tpl))
}
