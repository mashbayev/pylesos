// Файл: internal/tables/yaml.go
package tables

import "gopkg.in/yaml.v3"

// Marshal — шаблон в YAML (файл для импорта и экспорта шаблона).
func Marshal(t Template) ([]byte, error) { return yaml.Marshal(t.Normalize()) }

// Parse читает шаблон из YAML, нормализует и проверяет.
func Parse(data []byte) (Template, error) {
	var t Template
	if err := yaml.Unmarshal(data, &t); err != nil {
		return Template{}, err
	}
	t = t.Normalize()
	if err := t.Validate(); err != nil {
		return Template{}, err
	}
	return t, nil
}
