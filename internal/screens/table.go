// Файл: internal/screens/table.go
package screens

import (
	"context"
	"errors"
	"fmt"

	tdomain "pylesos/internal/table/domain"
	tservice "pylesos/internal/table/service"
	"pylesos/internal/tables"
	"pylesos/internal/texts"
)

// TableConfig — колонки таблицы (table/service.Config).
type TableConfig interface {
	Get(ctx context.Context) (tdomain.Config, error)
	Save(ctx context.Context, cfg tdomain.Config) error
}

func (b *Builder) Table(ctx context.Context) (TableScreen, error) {
	cfg, err := b.d.Table.Get(ctx)
	if err != nil {
		return TableScreen{}, fmt.Errorf("таблица: %w", err)
	}
	return tableScreen(cfg, "", ""), nil
}

// TableSave — сохранить колонки; ошибка ввода возвращается на экран вместе с введённым.
func (b *Builder) TableSave(ctx context.Context, cols []TableColumn, includeQueries bool) (TableScreen, error) {
	cfg := tdomain.Config{IncludeQueries: includeQueries}
	for _, c := range cols {
		cfg.Columns = append(cfg.Columns, tables.Column{Name: c.Name, Type: tables.ColumnType(c.Type), Hint: c.Hint})
	}
	var p *tservice.ProblemError
	if err := b.d.Table.Save(ctx, cfg); errors.As(err, &p) {
		return tableScreen(cfg, "", p.Text), nil
	} else if err != nil {
		return TableScreen{}, fmt.Errorf("таблица: %w", err)
	}
	return b.tableSaved(ctx)
}

func (b *Builder) tableSaved(ctx context.Context) (TableScreen, error) {
	cfg, err := b.d.Table.Get(ctx)
	if err != nil {
		return TableScreen{}, fmt.Errorf("таблица: %w", err)
	}
	return tableScreen(cfg, texts.TableSaved, ""), nil
}

func tableScreen(cfg tdomain.Config, message, problem string) TableScreen {
	s := TableScreen{Title: texts.TableTitle, Hint: texts.TableHint, Columns: []TableColumn{},
		NewColumn: TableColumn{Type: string(tables.TypeText)}, NameLabel: texts.TableColName, TypeLabel: texts.TableColType,
		HintLabel: texts.TableColHint, AddLabel: texts.TableAddColumn, RemoveLabel: texts.TableRemove,
		IncludeQueries: cfg.IncludeQueries, IncludeLabel: texts.TableIncludeQueries, SaveLabel: texts.TableSave,
		SavingLabel: texts.TableSaving, Message: message, Error: problem}
	for _, c := range cfg.Columns {
		s.Columns = append(s.Columns, TableColumn{Name: c.Name, Type: string(c.Type), Hint: c.Hint})
	}
	for _, t := range tservice.Types() {
		s.Types = append(s.Types, Option2{ID: string(t), Label: typeLabel(t)})
	}
	return s
}

func typeLabel(t tables.ColumnType) string {
	switch t {
	case tables.TypeNumber:
		return texts.TypeNumber
	case tables.TypeDate:
		return texts.TypeDate
	case tables.TypeIIN:
		return texts.TypeIIN
	case tables.TypePhone:
		return texts.TypePhone
	}
	return texts.TypeText
}
