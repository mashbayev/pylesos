// Файл: internal/export/excel_test.go
package export

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"

	"pylesos/internal/tables"
)

func excelTpl() tables.Template {
	return tables.Template{Name: "Водители/2024", Mode: tables.ModeDocument, Columns: []tables.Column{
		{Name: "ФИО", Type: tables.TypeText},
		{Name: "ИИН", Type: tables.TypeIIN},
		{Name: "Сумма", Type: tables.TypeMoney},
		{Name: "Дата", Type: tables.TypeDate},
		{Name: "Есть права", Type: tables.TypeBool},
	}}
}

func excelRows() []tables.Row {
	return []tables.Row{
		{File: "Жандос/права.pdf", Page: 0, Cells: map[string]tables.Cell{
			"ФИО":        {Value: "Әбдіхалықов Нұрсұлтан", Typed: "Әбдіхалықов Нұрсұлтан", Confidence: "high"},
			"ИИН":        {Value: "050315400124", Typed: "050315400124", Confidence: "high"},
			"Сумма":      {Value: "1250000.50", Typed: 1250000.50, Confidence: "high"},
			"Дата":       {Value: "12.03.2024", Typed: time.Date(2024, 3, 12, 0, 0, 0, 0, time.UTC), Confidence: "high"},
			"Есть права": {Value: "да", Typed: true, Confidence: "high"},
		}},
		{File: "Асқар/скан.jpg", Page: 2, Cells: map[string]tables.Cell{
			"ФИО": {Value: "Асқар", Typed: "Асқар", Confidence: "low"},
			"ИИН": {Value: "850315400125", Typed: "850315400125", Confidence: "high", Issues: []string{"chk.iin_checksum"}},
		}},
	}
}

func openXLSX(t *testing.T, path string) *excelize.File {
	t.Helper()
	f, err := excelize.OpenFile(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := f.Close(); err != nil {
			t.Errorf("закрыть xlsx: %v", err)
		}
	})
	return f
}

func cell(t *testing.T, f *excelize.File, sheet, ref string) string {
	t.Helper()
	v, err := f.GetCellValue(sheet, ref)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func fillColor(t *testing.T, f *excelize.File, sheet, ref string) string {
	t.Helper()
	id, err := f.GetCellStyle(sheet, ref)
	if err != nil {
		t.Fatal(err)
	}
	st, err := f.GetStyle(id)
	if err != nil || len(st.Fill.Color) == 0 {
		return ""
	}
	return strings.ToUpper(st.Fill.Color[0])
}

func TestWriteExcelRussian(t *testing.T) {
	out := t.TempDir()
	report := []FileReport{{File: "Жандос/права.pdf", Pages: 2}, {File: "битый.pdf", Pages: 0, Error: "Не удалось открыть PDF"}}
	path, err := WriteExcel(out, excelTpl(), excelRows(), report)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(path) != "Водители_2024.xlsx" {
		t.Fatalf("имя файла %q", path)
	}
	f := openXLSX(t, path)
	if got := f.GetSheetList(); len(got) != 2 || got[0] != "Данные" || got[1] != "Отчёт" {
		t.Fatalf("листы %v", got)
	}
	checkHeaders(t, f)
	checkTypedCells(t, f)
	checkReview(t, f)
	if cell(t, f, "Отчёт", "A1") != "Файл" || cell(t, f, "Отчёт", "A3") != "битый.pdf" || cell(t, f, "Отчёт", "C3") != "ошибка" || cell(t, f, "Отчёт", "D3") != "Не удалось открыть PDF" || cell(t, f, "Отчёт", "C2") != "готово" {
		t.Fatal("лист отчёта")
	}
	entries, err := os.ReadDir(out)
	if err != nil || len(entries) != 1 {
		t.Fatalf("лишние файлы: %d %v", len(entries), err)
	}
}

func checkHeaders(t *testing.T, f *excelize.File) {
	t.Helper()
	headers := []string{"ФИО", "ИИН", "Сумма", "Дата", "Есть права", "Файл", "Страница", "Требует проверки"}
	for i, h := range headers {
		ref, err := excelize.CoordinatesToCellName(i+1, 1)
		if err != nil {
			t.Fatal(err)
		}
		if got := cell(t, f, "Данные", ref); got != h {
			t.Fatalf("заголовок %s = %q, want %q", ref, got, h)
		}
	}
}

func checkTypedCells(t *testing.T, f *excelize.File) {
	t.Helper()
	if cell(t, f, "Данные", "B2") != "050315400124" {
		t.Fatal("ИИН с ведущим нулём должен сохраниться текстом")
	}
	if got := cell(t, f, "Данные", "C2"); got != "1,250,000.50" {
		t.Fatalf("сумма %q", got)
	}
	if got := cell(t, f, "Данные", "D2"); got != "12.03.2024" {
		t.Fatalf("дата %q", got)
	}
	if got := cell(t, f, "Данные", "E2"); got != "TRUE" {
		t.Fatalf("да/нет %q", got)
	}
	if cell(t, f, "Данные", "F2") != "Жандос/права.pdf" || cell(t, f, "Данные", "G2") != "" || cell(t, f, "Данные", "G3") != "2" {
		t.Fatal("служебные колонки Файл/Страница")
	}
	if cell(t, f, "Данные", "H2") != "" {
		t.Fatal("чистая строка не требует проверки")
	}
}

func checkReview(t *testing.T, f *excelize.File) {
	t.Helper()
	review := cell(t, f, "Данные", "H3")
	if !strings.Contains(review, "ФИО: модель не уверена в значении") || !strings.Contains(review, "ИИН: неверная контрольная цифра ИИН") {
		t.Fatalf("требует проверки: %q", review)
	}
	if fillColor(t, f, "Данные", "A3") != "FFF2CC" || fillColor(t, f, "Данные", "B3") != "FFF2CC" || fillColor(t, f, "Данные", "A2") != "" {
		t.Fatal("сомнительные ячейки должны быть жёлтыми, остальные — без заливки")
	}
	comments, err := f.GetComments("Данные")
	if err != nil || len(comments) != 2 {
		t.Fatalf("примечаний %d %v", len(comments), err)
	}
}

func TestWriteExcelFallsBackWhenFileIsLocked(t *testing.T) {
	out := t.TempDir()
	// Файл «занят»: на его месте папка — переименование поверх неё не удастся, как и поверх
	// файла, открытого в Excel на Windows.
	if err := os.Mkdir(filepath.Join(out, "Водители_2024.xlsx"), 0o755); err != nil {
		t.Fatal(err)
	}
	path, err := WriteExcel(out, excelTpl(), excelRows(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(path) != "Водители_2024 (2).xlsx" {
		t.Fatalf("путь %q", path)
	}
	openXLSX(t, path)
}
