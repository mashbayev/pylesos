// Файл: internal/validate/validate_test.go
package validate

import (
	"reflect"
	"testing"
	"time"

	"pylesos/internal/tables"
)

func col(t tables.ColumnType, required bool, options ...string) tables.Column {
	return tables.Column{Name: "x", Type: t, Required: required, Options: options}
}

// checkCase — случай проверки значения.
type checkCase struct {
	name   string
	col    tables.Column
	raw    string
	value  string
	typed  any
	issues []string
}

func date(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

func checkCasesA() []checkCase {
	return []checkCase{
		{"пусто необязательное", col(tables.TypeText, false), "  ", "", nil, nil},
		{"пусто обязательное", col(tables.TypeText, true), "", "", nil, []string{"chk.required"}},
		{"текст", col(tables.TypeText, false), " Әбдіхалықов ", "Әбдіхалықов", "Әбдіхалықов", nil},
		{"комментарий без проверки алфавита", col(tables.TypeComment, false), "Тестовый Автопарк: Testov есть инфо?", "Тестовый Автопарк: Testov есть инфо?", "Тестовый Автопарк: Testov есть инфо?", nil},
		{"смешение алфавитов", col(tables.TypeText, false), "Абдыхaлыков", "Абдыхaлыков", "Абдыхaлыков", []string{"chk.mixed_script"}},
		{"неразборчиво", col(tables.TypeText, false), "Нұр[неразборчиво]", "Нұр[неразборчиво]", "Нұр[неразборчиво]", []string{"chk.unclear"}},
		{"неуверенное слово", col(tables.TypeText, false), "[?Өмірұлы]", "[?Өмірұлы]", "[?Өмірұлы]", []string{"chk.unclear"}},

		{"число с пробелами", col(tables.TypeNumber, false), "1 250 000", "1250000", 1250000.0, nil},
		{"число с запятой", col(tables.TypeNumber, false), "12,5", "12.5", 12.5, nil},
		{"число неразрывные пробелы", col(tables.TypeNumber, false), "3 000,75", "3000.75", 3000.75, nil},
		{"тысячи через запятые", col(tables.TypeNumber, false), "1,250,000", "1250000", 1250000.0, nil},
		{"не число", col(tables.TypeNumber, false), "двенадцать", "двенадцать", "двенадцать", []string{"chk.number"}},

		{"неоднозначная запятая", col(tables.TypeNumber, false), "1,250", "1.25", 1.25, []string{"chk.number_ambiguous"}},
		{"неоднозначная точка в сумме", col(tables.TypeMoney, false), "1.250 тг", "1.25", 1.25, []string{"chk.number_ambiguous"}},
		{"пробелы снимают неоднозначность", col(tables.TypeNumber, false), "1 250,500", "1250.5", 1250.5, nil},
		{"две цифры после запятой", col(tables.TypeNumber, false), "12,50", "12.5", 12.5, nil},
		{"шестнадцатеричное", col(tables.TypeNumber, false), "0x1p4", "0x1p4", "0x1p4", []string{"chk.number"}},
		{"экспонента", col(tables.TypeNumber, false), "1e3", "1e3", "1e3", []string{"chk.number"}},
		{"сумма в тенге", col(tables.TypeMoney, false), "1 250 000,50 тг", "1250000.50", 1250000.50, nil},
		{"сумма со знаком", col(tables.TypeMoney, false), "₸ 700", "700.00", 700.0, nil},
		{"сумма KZT", col(tables.TypeMoney, false), "15000 KZT", "15000.00", 15000.0, nil},

		{"дата цифрами", col(tables.TypeDate, false), "12.03.2024", "12.03.2024", date(2024, 3, 12), nil},
		{"дата ISO", col(tables.TypeDate, false), "2024-03-12", "12.03.2024", date(2024, 3, 12), nil},
		{"дата короткий год", col(tables.TypeDate, false), "5/3/24", "05.03.2024", date(2024, 3, 5), nil},
	}
}

func checkCasesB() []checkCase {
	return []checkCase{
		{"дата по-русски", col(tables.TypeDate, false), "12 марта 2024 г.", "12.03.2024", date(2024, 3, 12), nil},
		{"дата по-казахски", col(tables.TypeDate, false), "2024 жылғы 12 наурыз", "12.03.2024", date(2024, 3, 12), nil},
		{"дата по-казахски 2", col(tables.TypeDate, false), "1 қыркүйек 2019 ж.", "01.09.2019", date(2019, 9, 1), nil},
		{"несуществующая дата", col(tables.TypeDate, false), "31.02.2024", "31.02.2024", "31.02.2024", []string{"chk.date"}},
		{"не дата", col(tables.TypeDate, false), "вчера", "вчера", "вчера", []string{"chk.date"}},

		{"ИИН верный", col(tables.TypeIIN, false), "850315 400124", "850315400124", "850315400124", nil},
		{"ИИН 2000-х", col(tables.TypeIIN, false), "051106500774", "051106500774", "051106500774", nil},
		{"ИИН опечатка", col(tables.TypeIIN, false), "850315400125", "850315400125", "850315400125", []string{"chk.iin_checksum"}},
		{"ИИН короткий", col(tables.TypeIIN, false), "85031540012", "85031540012", "85031540012", []string{"chk.iin_format"}},
		{"ИИН с буквой", col(tables.TypeIIN, false), "85031540O124", "85031540O124", "85031540O124", []string{"chk.iin_format"}},
		{"ИИН несуществующая дата", col(tables.TypeIIN, false), "851340400123", "851340400123", "851340400123", []string{"chk.iin_format"}},
		{"БИН верный", col(tables.TypeBIN, false), "120340123456", "120340123456", "120340123456", nil},
		{"БИН опечатка", col(tables.TypeBIN, false), "120340123457", "120340123457", "120340123457", []string{"chk.bin_checksum"}},

		{"телефон 8", col(tables.TypePhone, false), "8 (701) 123-45-67", "+7 701 123 45 67", "+7 701 123 45 67", nil},
		{"телефон 10 цифр", col(tables.TypePhone, false), "7011234567", "+7 701 123 45 67", "+7 701 123 45 67", nil},
		{"телефон +7", col(tables.TypePhone, false), "+77011234567", "+7 701 123 45 67", "+7 701 123 45 67", nil},
		{"не телефон", col(tables.TypePhone, false), "12-34", "12-34", "12-34", []string{"chk.phone"}},

		{"да", col(tables.TypeBool, false), "Иә", "да", true, nil},
		{"нет", col(tables.TypeBool, false), "жоқ", "нет", false, nil},
		{"не да/нет", col(tables.TypeBool, false), "может быть", "может быть", "может быть", []string{"chk.bool"}},

		{"выбор", col(tables.TypeChoice, false, "A", "B", "BE"), "b", "B", "B", nil},
		{"выбор несколько, кириллица", col(tables.TypeChoice, false, "A", "B", "BE", "C"), "В, ВЕ; С", "B, BE, C", "B, BE, C", nil},
		{"выбор не из списка", col(tables.TypeChoice, false, "A", "B"), "B, Z", "B, Z", "B, Z", []string{"chk.choice"}},
	}
}

func TestCheck(t *testing.T) {
	cases := append(checkCasesA(), checkCasesB()...)
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := Check(c.col, c.raw)
			if r.Value != c.value || !reflect.DeepEqual(r.Typed, c.typed) || !reflect.DeepEqual(r.Issues, c.issues) {
				t.Fatalf("got value %q typed %#v issues %v; want %q %#v %v", r.Value, r.Typed, r.Issues, c.value, c.typed, c.issues)
			}
		})
	}
}

func TestIssueKeysListed(t *testing.T) {
	if len(IssueKeys()) < 12 {
		t.Fatalf("IssueKeys(): %v", IssueKeys())
	}
}
