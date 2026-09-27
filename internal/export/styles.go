// Файл: internal/export/styles.go
package export

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/xuri/excelize/v2"

	"pylesos/internal/tables"
)

const yellow = "FFF2CC"

type styles struct {
	header                                  int
	yellow, money, moneyYellow, date, dateY int
}

func newStyles(f *excelize.File) (*styles, error) {
	moneyFmt, dateFmt := "#,##0.00", "dd.mm.yyyy"
	fill := excelize.Fill{Type: "pattern", Color: []string{yellow}, Pattern: 1}
	defs := []*excelize.Style{
		{Font: &excelize.Font{Bold: true}, Fill: excelize.Fill{Type: "pattern", Color: []string{"E7E6E6"}, Pattern: 1}},
		{Fill: fill},
		{CustomNumFmt: &moneyFmt},
		{CustomNumFmt: &moneyFmt, Fill: fill},
		{CustomNumFmt: &dateFmt},
		{CustomNumFmt: &dateFmt, Fill: fill},
	}
	ids := make([]int, len(defs))
	for i, d := range defs {
		id, err := f.NewStyle(d)
		if err != nil {
			return nil, err
		}
		ids[i] = id
	}
	return &styles{header: ids[0], yellow: ids[1], money: ids[2], moneyYellow: ids[3], date: ids[4], dateY: ids[5]}, nil
}

func (s *styles) forType(t tables.ColumnType, flagged bool) int {
	switch {
	case t == tables.TypeMoney && flagged:
		return s.moneyYellow
	case t == tables.TypeMoney:
		return s.money
	case t == tables.TypeDate && flagged:
		return s.dateY
	case t == tables.TypeDate:
		return s.date
	case flagged:
		return s.yellow
	}
	return 0
}

// save пишет книгу во временный файл и переносит под именем name. Прежний файл может быть открыт
// в Excel (на Windows он заблокирован) — тогда «<имя> (2).xlsx», «(3)» и т.д., а не потеря результата.
func save(f *excelize.File, outDir, name string) (string, error) {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(outDir, ".tmp-*.xlsx")
	if err != nil {
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", errors.Join(err, os.Remove(tmp.Name()))
	}
	if err := f.SaveAs(tmp.Name()); err != nil {
		return "", errors.Join(err, os.Remove(tmp.Name()))
	}
	path := filepath.Join(outDir, name)
	base := strings.TrimSuffix(path, ".xlsx")
	for n := 1; n <= 20; n++ {
		candidate := path
		if n > 1 {
			candidate = fmt.Sprintf("%s (%d).xlsx", base, n)
		}
		if os.Rename(tmp.Name(), candidate) == nil {
			return candidate, nil
		}
	}
	return "", errors.Join(ErrExcelLocked, os.Remove(tmp.Name()))
}
