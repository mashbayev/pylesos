// Файл: internal/table/service/config.go
// Package service — колонки таблицы (конструктор, П29) и задание выгрузки людей в Excel.
package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"pylesos/internal/table/domain"
	"pylesos/internal/tables"
	"pylesos/internal/texts"
)

// Store — хранилище колонок и готовых строк (table/repository).
type Store interface {
	Config(ctx context.Context) (domain.Config, bool, error)
	SaveConfig(ctx context.Context, cfg domain.Config) error
	Row(ctx context.Context, personID int64, hash string) (tables.Row, bool, error)
	SaveRow(ctx context.Context, personID int64, hash string, row tables.Row) error
	People(ctx context.Context, sourceID int64, includeQueries bool) ([]domain.Person, error)
	DocTexts(ctx context.Context, personID int64) ([]string, error)
}

type Config struct{ store Store }

func NewConfig(store Store) *Config { return &Config{store: store} }

// Types — типы колонок в конструкторе (без выбора из списка — MVP).
func Types() []tables.ColumnType {
	return []tables.ColumnType{tables.TypeText, tables.TypeNumber, tables.TypeDate, tables.TypeIIN, tables.TypePhone}
}

// defaultColumns — колонки до первого сохранения: опознавательные поля документа.
func defaultColumns() []tables.Column {
	return []tables.Column{
		{Name: "ФИО", Type: tables.TypeText, Hint: "фамилия, имя, отчество полностью, как в документе"},
		{Name: "ИИН", Type: tables.TypeIIN, Hint: "12 цифр"},
		{Name: "Дата рождения", Type: tables.TypeDate},
		{Name: "Номер документа", Type: tables.TypeText, Hint: "номер удостоверения или водительских прав"},
	}
}

func (c *Config) Get(ctx context.Context) (domain.Config, error) {
	cfg, ok, err := c.store.Config(ctx)
	if err != nil {
		return domain.Config{}, err
	}
	if !ok {
		cfg.Columns = defaultColumns()
	}
	return cfg, nil
}

// ProblemError — ошибка ввода колонок: готовый текст для экрана.
type ProblemError struct{ Text string }

func (e *ProblemError) Error() string { return e.Text }

// Save проверяет колонки и сохраняет; ошибка ввода — *ProblemError.
func (c *Config) Save(ctx context.Context, cfg domain.Config) error {
	tpl := Template("", cfg.Columns)
	cfg.Columns = tpl.Columns
	if text := problem(tpl); text != "" {
		return &ProblemError{Text: text}
	}
	return c.store.SaveConfig(ctx, cfg)
}

// Template — шаблон извлечения по колонкам: одна строка на человека.
func Template(name string, cols []tables.Column) tables.Template {
	return tables.Template{Name: name, Mode: tables.ModeDocument, Columns: cols}.Normalize()
}

func problem(tpl tables.Template) string {
	tpl.Name = "-"
	for _, col := range tpl.Columns {
		if strings.EqualFold(col.Name, texts.TableComments) {
			return fmt.Sprintf(texts.ErrTableColReserved, col.Name)
		}
		if col.Type == tables.TypeChoice || col.Type == tables.TypeComment {
			return fmt.Sprintf(texts.ErrTableColType, col.Name)
		}
	}
	var ve *tables.ValidationError
	if err := tpl.Validate(); errors.As(err, &ve) {
		switch ve.Key {
		case "tpl.no_columns":
			return texts.ErrTableNoColumns
		case "tpl.column_no_name":
			return fmt.Sprintf(texts.ErrTableColNoName, ve.Arg)
		case "tpl.column_duplicate":
			return fmt.Sprintf(texts.ErrTableColDup, ve.Arg)
		case "tpl.column_reserved":
			return fmt.Sprintf(texts.ErrTableColReserved, ve.Arg)
		default:
			return fmt.Sprintf(texts.ErrTableColType, ve.Arg)
		}
	}
	return ""
}
