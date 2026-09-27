// Файл: internal/docforms/domain/form.go
// Package domain — формы документов (П21): образец, рамка карточки и зоны опознавательных полей.
// Зоны принадлежат форме, а не шаблону таблицы; поля формы постоянные (спецификация, раздел 4, шаг 1).
package domain

import (
	"errors"

	"pylesos/internal/tables"
)

// Опознавательные поля документа.
const (
	FieldFullName  = "full_name"
	FieldIIN       = "iin"
	FieldDocNumber = "doc_number"
	FieldBirthDate = "birth_date"
	// FrameKey — не поле, а рамка карточки документа (для поиска документа на скрине переписки).
	FrameKey = "__frame__"
)

// Fields — поля формы по порядку.
func Fields() []string { return []string{FieldFullName, FieldIIN, FieldDocNumber, FieldBirthDate} }

// FieldColumn — поле как колонка шаблона: название для модели и тип для проверки значения.
func FieldColumn(field string) (tables.Column, bool) {
	switch field {
	case FieldFullName:
		return tables.Column{Name: "ФИО", Type: tables.TypeText, Hint: "фамилия, имя, отчество полностью, как в документе"}, true
	case FieldIIN:
		return tables.Column{Name: "ИИН", Type: tables.TypeIIN, Hint: "12 цифр"}, true
	case FieldDocNumber:
		return tables.Column{Name: "Номер документа", Type: tables.TypeText, Hint: "номер удостоверения или водительских прав"}, true
	case FieldBirthDate:
		return tables.Column{Name: "Дата рождения", Type: tables.TypeDate}, true
	}
	return tables.Column{}, false
}

// FieldLabel — название поля (как колонки).
func FieldLabel(field string) string {
	c, ok := FieldColumn(field)
	if !ok {
		return field
	}
	return c.Name
}

type Form struct {
	ID        int64
	Name      string
	Sample    []byte // JPEG образца
	Signature string // отпечаток образца (recognize.Signature)
	Frame     *tables.Zone
	Locator   string                 // образец карточки для поиска на скрине (recognize.Locator)
	Zones     map[string]tables.Zone // поле → зона
	UpdatedAt int64
}

// AsTablesForm — форма в виде, который понимает распознавание (зоны под названиями колонок).
func (f Form) AsTablesForm() tables.Form {
	zones := make(map[string]tables.Zone, len(f.Zones))
	for field, z := range f.Zones {
		zones[FieldLabel(field)] = z
	}
	return tables.Form{Name: f.Name, Signature: f.Signature, Zones: zones, Frame: f.Frame, Locator: f.Locator}
}

var (
	ErrNotFound = errors.New("форма документа не найдена")
	ErrNoName   = errors.New("у формы нет названия")
	ErrNoSample = errors.New("образец не читается как изображение")
	ErrBadZone  = errors.New("отметка вне образца или нулевого размера")
	ErrBadField = errors.New("неизвестное поле формы")
)
