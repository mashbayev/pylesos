// Файл: internal/tables/forms_test.go
package tables

import (
	"errors"
	"reflect"
	"testing"
)

func zonedTemplate() Template {
	t := validTemplate()
	t.Mode = ModeDocument
	t.Forms = []Form{
		{Name: "Удостоверение — лицевая", Signature: "sig1", Zones: map[string]Zone{"Наименование": {X: 0.1, Y: 0.2, W: 0.5, H: 0.05}}},
		{Name: "Права", Signature: "sig2", Zones: map[string]Zone{"Кол-во": {X: 0.5, Y: 0.5, W: 0.2, H: 0.1}}},
	}
	return t
}

func TestValidateForms(t *testing.T) {
	cases := []struct {
		name string
		edit func(*Template)
		key  string
	}{
		{"корректно", func(*Template) {}, ""},
		{"вид без названия", func(t *Template) { t.Forms[0].Name = " " }, "tpl.form_no_name"},
		{"повтор вида", func(t *Template) { t.Forms[1].Name = "удостоверение — лицевая" }, "tpl.form_duplicate"},
		{"зона чужой колонки", func(t *Template) { t.Forms[0].Zones["Нет такой"] = Zone{X: 0, Y: 0, W: 0.1, H: 0.1} }, "tpl.zone_unknown_column"},
		{"нулевая ширина", func(t *Template) { t.Forms[0].Zones["Наименование"] = Zone{X: 0.1, Y: 0.1, W: 0, H: 0.1} }, "tpl.zone_bad"},
		{"за краем страницы", func(t *Template) { t.Forms[0].Zones["Наименование"] = Zone{X: 0.8, Y: 0.1, W: 0.3, H: 0.1} }, "tpl.zone_bad"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tpl := zonedTemplate()
			c.edit(&tpl)
			err := tpl.Normalize().Validate()
			var ve *ValidationError
			if c.key == "" && err != nil || c.key != "" && (!errors.As(err, &ve) || ve.Key != c.key) {
				t.Fatalf("ожидался %q, получено %v", c.key, err)
			}
		})
	}
}

func TestFormsRoundTripAndHash(t *testing.T) {
	data, err := Marshal(zonedTemplate())
	if err != nil {
		t.Fatal(err)
	}
	back, err := Parse(data)
	if err != nil || !reflect.DeepEqual(back, zonedTemplate().Normalize()) {
		t.Fatalf("parse %+v err %v", back, err)
	}
	moved := zonedTemplate()
	moved.Forms[0].Zones["Наименование"] = Zone{X: 0.11, Y: 0.2, W: 0.5, H: 0.05}
	if moved.Hash() == zonedTemplate().Hash() {
		t.Fatal("сдвиг зоны должен менять отпечаток шаблона (кэш извлечения)")
	}
}

func TestValidateFrame(t *testing.T) {
	tpl := zonedTemplate()
	tpl.Forms[0].Frame = &Zone{X: 0.1, Y: 0.2, W: 0.6, H: 0.3}
	if err := tpl.Normalize().Validate(); err != nil {
		t.Fatalf("рамка допустима: %v", err)
	}
	tpl.Forms[0].Frame = &Zone{X: 0.9, Y: 0.2, W: 0.6, H: 0.3}
	var ve *ValidationError
	if err := tpl.Normalize().Validate(); !errors.As(err, &ve) || ve.Key != "tpl.zone_bad" {
		t.Fatalf("рамка за краем: %v", err)
	}
}
