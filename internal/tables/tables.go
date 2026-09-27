// Файл: internal/tables/tables.go
// Package tables — шаблоны таблиц для Excel (конструктор таблиц) и их хранение.
package tables

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// Mode — что считать строкой таблицы.
type Mode string

const (
	ModeDocument  Mode = "document"   // одна строка на документ
	ModeTableRows Mode = "table_rows" // каждая строка таблиц внутри документа
	ModePage      Mode = "page"       // одна строка на страницу
	ModePeople    Mode = "people"     // каждый человек на изображении (скрин, фото) — отдельная строка
)

// Modes — все режимы строк.
func Modes() []Mode { return []Mode{ModeDocument, ModeTableRows, ModePage, ModePeople} }

type ColumnType string

const (
	TypeText   ColumnType = "text"
	TypeNumber ColumnType = "number"
	TypeMoney  ColumnType = "money"
	TypeDate   ColumnType = "date"
	TypeIIN    ColumnType = "iin"
	TypeBIN    ColumnType = "bin"
	TypePhone  ColumnType = "phone"
	TypeBool   ColumnType = "bool"
	TypeChoice ColumnType = "choice"
	// TypeComment — сообщения переписки рядом с документом (скрины мессенджеров).
	TypeComment ColumnType = "comment"
)

// ColumnTypes — все типы колонок.
func ColumnTypes() []ColumnType {
	return []ColumnType{TypeText, TypeNumber, TypeMoney, TypeDate, TypeIIN, TypeBIN, TypePhone, TypeBool, TypeChoice, TypeComment}
}

// ReservedNames — служебные колонки Excel (совпадают с texts.Xlsx*).
func ReservedNames() []string {
	return []string{"Файл", "Страница", "Требует проверки"}
}

type Column struct {
	Name     string     `yaml:"name" json:"name"`
	Type     ColumnType `yaml:"type" json:"type"`
	Hint     string     `yaml:"hint,omitempty" json:"hint"`
	Required bool       `yaml:"required,omitempty" json:"required"`
	Options  []string   `yaml:"options,omitempty" json:"options"`
}

type Template struct {
	Name         string   `yaml:"name" json:"name"`
	Mode         Mode     `yaml:"mode" json:"mode"`
	Instructions string   `yaml:"instructions,omitempty" json:"instructions"`
	Columns      []Column `yaml:"columns" json:"columns"`
	Forms        []Form   `yaml:"forms,omitempty" json:"forms"`
}

// Zone — прямоугольник на странице в долях ширины и высоты (0…1).
type Zone struct {
	X float64 `yaml:"x" json:"x"`
	Y float64 `yaml:"y" json:"y"`
	W float64 `yaml:"w" json:"w"`
	H float64 `yaml:"h" json:"h"`
}

// Form — вид документа (например, «Удостоверение — лицевая»): отпечаток образца и зоны колонок.
type Form struct {
	Name      string          `yaml:"name" json:"name"`
	Signature string          `yaml:"signature" json:"signature"` // миниатюра образца 32×32, base64 PNG
	Zones     map[string]Zone `yaml:"zones" json:"zones"`
	// Frame — где на образце сама карточка документа (скрины переписки, где документ — часть
	// картинки). Если задана, документ ищется на странице по Locator, а зоны отсчитываются от рамки.
	Frame   *Zone  `yaml:"frame,omitempty" json:"frame,omitempty"`
	Locator string `yaml:"locator,omitempty" json:"locator,omitempty"` // карточка из образца, base64 PNG
}

// Valid — зона внутри страницы (доли 0…1) и ненулевой площади.
func (z Zone) Valid() bool { return z.valid() }

func (z Zone) valid() bool {
	return z.W > 0 && z.H > 0 && z.X >= 0 && z.Y >= 0 && z.X+z.W <= 1.0001 && z.Y+z.H <= 1.0001
}

// Cell — значение ячейки после извлечения и проверок.
type Cell struct {
	Value      string   `json:"value"`
	Confidence string   `json:"confidence"` // "high" или "low"
	Issues     []string `json:"issues,omitempty"`
	Typed      any      `json:"-"` // значение для Excel: float64, time.Time, bool или string
}

// Row — строка таблицы: откуда взята и значения по названиям колонок.
type Row struct {
	File  string          `json:"file"`
	Page  int             `json:"page"` // 0 — весь документ
	Cells map[string]Cell `json:"cells"`
}

// ValidationError — ошибка шаблона с i18n-ключом и аргументом (название колонки и т.п.).
type ValidationError struct {
	Key string
	Arg string
}

func (e *ValidationError) Error() string {
	if e.Arg == "" {
		return e.Key
	}
	return e.Key + ": " + e.Arg
}

// Normalize обрезает пробелы и подставляет значения по умолчанию.
func (t Template) Normalize() Template {
	out := Template{Name: strings.TrimSpace(t.Name), Mode: t.Mode, Instructions: strings.TrimSpace(t.Instructions)}
	if out.Mode == "" {
		out.Mode = ModeDocument
	}
	for _, c := range t.Columns {
		col := Column{Name: strings.TrimSpace(c.Name), Type: c.Type, Hint: strings.TrimSpace(c.Hint), Required: c.Required}
		if col.Type == "" {
			col.Type = TypeText
		}
		for _, o := range c.Options {
			if o = strings.TrimSpace(o); o != "" {
				col.Options = append(col.Options, o)
			}
		}
		out.Columns = append(out.Columns, col)
	}
	for _, f := range t.Forms {
		form := Form{Name: strings.TrimSpace(f.Name), Signature: f.Signature, Zones: map[string]Zone{}, Locator: f.Locator}
		if f.Frame != nil {
			frame := *f.Frame
			form.Frame = &frame
		}
		for name, z := range f.Zones {
			form.Zones[strings.TrimSpace(name)] = z
		}
		out.Forms = append(out.Forms, form)
	}
	return out
}

func (t Template) Validate() error {
	if strings.TrimSpace(t.Name) == "" {
		return &ValidationError{Key: "tpl.no_name"}
	}
	if !slices.Contains(Modes(), t.Mode) {
		return &ValidationError{Key: "tpl.bad_mode", Arg: string(t.Mode)}
	}
	if len(t.Columns) == 0 {
		return &ValidationError{Key: "tpl.no_columns"}
	}
	seen, err := validateColumns(t.Columns)
	if err != nil {
		return err
	}
	return validateForms(t.Forms, seen)
}

// validateColumns — имена колонок (в нижнем регистре) или первая ошибка.
func validateColumns(cols []Column) (map[string]bool, error) {
	seen := map[string]bool{}
	for i, c := range cols {
		name := strings.TrimSpace(c.Name)
		if name == "" {
			return nil, &ValidationError{Key: "tpl.column_no_name", Arg: fmt.Sprint(i + 1)}
		}
		lower := strings.ToLower(name)
		if seen[lower] {
			return nil, &ValidationError{Key: "tpl.column_duplicate", Arg: name}
		}
		seen[lower] = true
		if slices.ContainsFunc(ReservedNames(), func(r string) bool { return strings.EqualFold(r, name) }) {
			return nil, &ValidationError{Key: "tpl.column_reserved", Arg: name}
		}
		if !slices.Contains(ColumnTypes(), c.Type) {
			return nil, &ValidationError{Key: "tpl.column_bad_type", Arg: name}
		}
		if c.Type == TypeChoice && !slices.ContainsFunc(c.Options, func(o string) bool { return strings.TrimSpace(o) != "" }) {
			return nil, &ValidationError{Key: "tpl.choice_no_options", Arg: name}
		}
	}
	return seen, nil
}

func validateForms(forms []Form, columns map[string]bool) error {
	names := map[string]bool{}
	for _, f := range forms {
		name := strings.TrimSpace(f.Name)
		if name == "" {
			return &ValidationError{Key: "tpl.form_no_name"}
		}
		if names[strings.ToLower(name)] {
			return &ValidationError{Key: "tpl.form_duplicate", Arg: name}
		}
		names[strings.ToLower(name)] = true
		for col, z := range f.Zones {
			if !columns[strings.ToLower(strings.TrimSpace(col))] {
				return &ValidationError{Key: "tpl.zone_unknown_column", Arg: col}
			}
			if !z.valid() {
				return &ValidationError{Key: "tpl.zone_bad", Arg: col}
			}
		}
		if f.Frame != nil && !f.Frame.valid() {
			return &ValidationError{Key: "tpl.zone_bad", Arg: name}
		}
	}
	return nil
}

// Hash — отпечаток содержимого шаблона (для кэша извлечения).
func (t Template) Hash() string {
	b, err := json.Marshal(t.Normalize())
	if err != nil { // шаблон из строк и чисел всегда сериализуется; на всякий случай — отпечаток текста
		b = []byte(fmt.Sprintf("%#v", t.Normalize()))
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:8])
}

// Example — встроенный пример для папки без шаблонов.
func Example() Template {
	return Template{
		Name:         "Водительские документы",
		Mode:         ModeDocument,
		Instructions: "Удостоверения личности и водительские удостоверения Республики Казахстан.",
		Columns: []Column{
			{Name: "ФИО", Type: TypeText, Hint: "фамилия, имя, отчество полностью, как в документе", Required: true},
			{Name: "ИИН", Type: TypeIIN, Hint: "12 цифр", Required: true},
			{Name: "Дата рождения", Type: TypeDate},
			{Name: "Номер документа", Type: TypeText, Hint: "номер удостоверения или водительских прав"},
			{Name: "Дата выдачи", Type: TypeDate},
			{Name: "Действителен до", Type: TypeDate},
			{Name: "Категории", Type: TypeChoice, Hint: "категории водительских прав через запятую", Options: []string{"A", "A1", "B", "B1", "BE", "C", "C1", "CE", "C1E", "D", "D1", "DE", "D1E", "Tb", "Tm"}},
		},
	}
}
