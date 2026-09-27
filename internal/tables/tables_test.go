// Файл: internal/tables/tables_test.go
package tables

import (
	"errors"
	"reflect"
	"testing"
)

func validTemplate() Template {
	return Template{Name: "Накладные", Mode: ModeTableRows, Columns: []Column{
		{Name: "Наименование", Type: TypeText, Required: true},
		{Name: "Кол-во", Type: TypeNumber},
		{Name: "Категория", Type: TypeChoice, Options: []string{"A", "B"}},
	}}
}

func TestValidate(t *testing.T) {
	cases := []struct {
		name string
		edit func(*Template)
		key  string
	}{
		{"корректный", func(*Template) {}, ""},
		{"без названия", func(t *Template) { t.Name = "  " }, "tpl.no_name"},
		{"плохой режим", func(t *Template) { t.Mode = "rows" }, "tpl.bad_mode"},
		{"без колонок", func(t *Template) { t.Columns = nil }, "tpl.no_columns"},
		{"колонка без названия", func(t *Template) { t.Columns[1].Name = "" }, "tpl.column_no_name"},
		{"повтор названия", func(t *Template) { t.Columns[1].Name = " наименование " }, "tpl.column_duplicate"},
		{"служебное название", func(t *Template) { t.Columns[1].Name = "Страница" }, "tpl.column_reserved"},
		{"плохой тип", func(t *Template) { t.Columns[1].Type = "float" }, "tpl.column_bad_type"},
		{"выбор без вариантов", func(t *Template) { t.Columns[2].Options = []string{" "} }, "tpl.choice_no_options"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tpl := validTemplate()
			c.edit(&tpl)
			err := tpl.Normalize().Validate()
			var ve *ValidationError
			switch {
			case c.key == "" && err != nil:
				t.Fatalf("неожиданная ошибка %v", err)
			case c.key != "" && (!errors.As(err, &ve) || ve.Key != c.key):
				t.Fatalf("ожидался %s, получено %v", c.key, err)
			}
		})
	}
}

func TestNormalizeDefaults(t *testing.T) {
	tpl := Template{Name: " Справки ", Columns: []Column{{Name: " ФИО "}, {Name: "Тип", Type: TypeChoice, Options: []string{" A ", "", "B"}}}}.Normalize()
	if tpl.Name != "Справки" || tpl.Mode != ModeDocument || tpl.Columns[0].Name != "ФИО" || tpl.Columns[0].Type != TypeText {
		t.Fatalf("normalize %+v", tpl)
	}
	if !reflect.DeepEqual(tpl.Columns[1].Options, []string{"A", "B"}) {
		t.Fatalf("options %v", tpl.Columns[1].Options)
	}
}

func TestParseAndHash(t *testing.T) {
	data, err := Marshal(validTemplate())
	if err != nil {
		t.Fatal(err)
	}
	back, err := Parse(data)
	if err != nil || !reflect.DeepEqual(back, validTemplate().Normalize()) {
		t.Fatalf("parse %+v err %v", back, err)
	}
	if _, err := Parse([]byte("name: x\nmode: document\ncolumns: []\n")); err == nil {
		t.Fatal("шаблон без колонок должен отклоняться")
	}
	a, b := validTemplate(), validTemplate()
	b.Columns[0].Hint = "другая подсказка"
	if a.Hash() == b.Hash() || a.Hash() != validTemplate().Hash() {
		t.Fatal("Hash должен зависеть от содержимого и быть стабильным")
	}
}

func TestPeopleModeAndCommentType(t *testing.T) {
	tpl := Template{Name: "ЧС", Mode: ModePeople, Columns: []Column{{Name: "ФИО"}, {Name: "Комментарий", Type: TypeComment}}}
	if err := tpl.Normalize().Validate(); err != nil {
		t.Fatalf("режим people и тип comment должны быть допустимы: %v", err)
	}
}
