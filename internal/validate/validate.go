// Файл: internal/validate/validate.go
// Package validate проверяет и нормализует значения ячеек по типу колонки.
package validate

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"pylesos/internal/tables"
)

// IssueKeys — все ключи проблем (тексты — texts.CheckIssue, подсветка в Excel).
func IssueKeys() []string {
	return []string{
		"chk.required", "chk.unclear", "chk.mixed_script", "chk.number", "chk.date",
		"chk.iin_format", "chk.iin_checksum", "chk.bin_format", "chk.bin_checksum",
		"chk.phone", "chk.bool", "chk.choice", "chk.low_confidence", "chk.number_ambiguous",
	}
}

// Result — нормализованное значение, значение для Excel и найденные проблемы.
type Result struct {
	Value  string
	Typed  any // float64, time.Time, bool или string; nil для пустого значения
	Issues []string
}

func unclearRe() *regexp.Regexp {
	return regexp.MustCompile(`\[неразборчиво\]|\[\?[^\]]*\]`)
}

func Check(col tables.Column, raw string) Result {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		if col.Required {
			return Result{Issues: []string{"chk.required"}}
		}
		return Result{}
	}
	r := checkType(col, raw)
	if unclearRe().MatchString(raw) {
		r.Issues = append(r.Issues, "chk.unclear")
	}
	return r
}

func bad(raw, issue string) Result { return Result{Value: raw, Typed: raw, Issues: []string{issue}} }

func checkType(col tables.Column, raw string) Result {
	switch col.Type {
	case tables.TypeNumber:
		f, ok, ambiguous := parseNumber(raw)
		if !ok {
			return bad(raw, "chk.number")
		}
		return numberResult(strconv.FormatFloat(f, 'f', -1, 64), f, ambiguous)
	case tables.TypeMoney:
		f, ok, ambiguous := parseNumber(currencyRe().ReplaceAllString(raw, ""))
		if !ok {
			return bad(raw, "chk.number")
		}
		return numberResult(fmt.Sprintf("%.2f", f), f, ambiguous)
	case tables.TypeDate:
		d, ok := parseDate(raw)
		if !ok {
			return bad(raw, "chk.date")
		}
		return Result{Value: d.Format("02.01.2006"), Typed: d}
	case tables.TypeIIN, tables.TypeBIN:
		return checkIIN(raw, col.Type == tables.TypeIIN)
	case tables.TypePhone:
		return checkPhone(raw)
	case tables.TypeBool:
		switch strings.ToLower(raw) {
		case "да", "иә", "yes", "true", "+", "1", "есть", "бар":
			return Result{Value: "да", Typed: true}
		case "нет", "жоқ", "no", "false", "-", "0", "отсутствует":
			return Result{Value: "нет", Typed: false}
		}
		return bad(raw, "chk.bool")
	case tables.TypeChoice:
		return checkChoice(raw, col.Options)
	case tables.TypeComment:
		return Result{Value: raw, Typed: raw} // в переписке кириллица и латиница смешиваются законно
	}
	r := Result{Value: raw, Typed: raw}
	if mixedScript(raw) {
		r.Issues = append(r.Issues, "chk.mixed_script")
	}
	return r
}

func numberResult(value string, f float64, ambiguous bool) Result {
	r := Result{Value: value, Typed: f}
	if ambiguous {
		r.Issues = []string{"chk.number_ambiguous"}
	}
	return r
}

func ambiguousRe() *regexp.Regexp { return regexp.MustCompile(`^-?\d{1,3}[.,]\d{3}$`) }

// parseNumber разбирает число с пробелами/неразрывными пробелами как разделителями тысяч.
// ambiguous — «1.250» / «1,250»: единственный разделитель и ровно три цифры после него —
// то ли тысячи, то ли дробь; значение читается как дробь, но ячейка помечается.
func parseNumber(s string) (f float64, ok, ambiguous bool) {
	hadSpace := false
	s = strings.Map(func(r rune) rune {
		switch {
		case unicode.IsSpace(r), r == '\'':
			hadSpace = true
			return -1
		case r == '−':
			return '-'
		}
		return r
	}, strings.TrimSpace(s))
	if s == "" || strings.IndexFunc(s, func(r rune) bool { return !strings.ContainsRune("0123456789.,-+", r) }) >= 0 ||
		!strings.ContainsAny(s, "0123456789") {
		return 0, false, false
	}
	ambiguous = !hadSpace && ambiguousRe().MatchString(s)
	comma, dot := strings.Count(s, ","), strings.Count(s, ".")
	switch {
	case comma > 0 && dot > 0: // последний разделитель — десятичный
		if strings.LastIndex(s, ",") > strings.LastIndex(s, ".") {
			s = strings.Replace(strings.ReplaceAll(s, ".", ""), ",", ".", 1)
		} else {
			s = strings.ReplaceAll(s, ",", "")
		}
	case comma > 1:
		s = strings.ReplaceAll(s, ",", "")
	case comma == 1:
		s = strings.Replace(s, ",", ".", 1)
	case dot > 1:
		s = strings.ReplaceAll(s, ".", "")
	}
	v, err := strconv.ParseFloat(s, 64)
	return v, err == nil, ambiguous
}

func currencyRe() *regexp.Regexp {
	return regexp.MustCompile(`(?i)тенге|теңге|тг\.?|₸|kzt|руб\.?|₽|\$|usd|€|eur`)
}

func dmyRe() *regexp.Regexp {
	return regexp.MustCompile(`^(\d{1,2})[./\-](\d{1,2})[./\-](\d{4}|\d{2})$`)
}
func ymdRe() *regexp.Regexp { return regexp.MustCompile(`^(\d{4})[./\-](\d{1,2})[./\-](\d{1,2})$`) }
func yearWordRe() *regexp.Regexp {
	return regexp.MustCompile(`(?i)\s*(года|год|г\.?|жылғы|жылы|жыл|ж\.?)$`)
}

// monthStem — основа названия месяца.
type monthStem struct {
	stem  string
	month time.Month
}

// monthStems — основы названий месяцев (русские падежи и казахские).
func monthStems() []monthStem {
	return []monthStem{
		{"қаңтар", 1}, {"ақпан", 2}, {"наурыз", 3}, {"сәуір", 4}, {"мамыр", 5}, {"маусым", 6},
		{"шілде", 7}, {"тамыз", 8}, {"қыркүйек", 9}, {"қазан", 10}, {"қараша", 11}, {"желтоқсан", 12},
		{"январ", 1}, {"феврал", 2}, {"март", 3}, {"апрел", 4}, {"мая", 5}, {"май", 5}, {"июн", 6},
		{"июл", 7}, {"август", 8}, {"сентябр", 9}, {"октябр", 10}, {"ноябр", 11}, {"декабр", 12},
	}
}

func parseDate(raw string) (time.Time, bool) {
	s := strings.ToLower(yearWordRe().ReplaceAllString(strings.TrimSpace(raw), ""))
	var y, m, d int
	if g := dmyRe().FindStringSubmatch(s); g != nil {
		d, m, y = atoi(g[1]), atoi(g[2]), atoi(g[3])
		if len(g[3]) == 2 {
			y = fullYear(y)
		}
	} else if g := ymdRe().FindStringSubmatch(s); g != nil {
		y, m, d = atoi(g[1]), atoi(g[2]), atoi(g[3])
	} else {
		for _, tok := range strings.FieldsFunc(s, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
			if n, err := strconv.Atoi(tok); err == nil {
				if len(tok) == 4 {
					y = n
				} else if d == 0 {
					d = n
				}
				continue
			}
			for _, ms := range monthStems() {
				if m == 0 && strings.HasPrefix(tok, ms.stem) {
					m = int(ms.month)
				}
			}
		}
	}
	return validDate(y, m, d)
}

func fullYear(yy int) int {
	if yy <= time.Now().Year()%100+1 {
		return 2000 + yy
	}
	return 1900 + yy
}

func validDate(y, m, d int) (time.Time, bool) {
	if y == 0 || m < 1 || m > 12 || d < 1 {
		return time.Time{}, false
	}
	t := time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC)
	return t, t.Day() == d && int(t.Month()) == m
}

// atoi — число из строки цифр (после проверки регулярным выражением); нечисло — 0.
func atoi(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
}

// checkIIN: 12 цифр, контрольный разряд РК; у ИИН первые 6 цифр — дата рождения (век — по 7-й цифре).
func checkIIN(raw string, iin bool) Result {
	digits := strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || r == '-' {
			return -1
		}
		return r
	}, raw)
	format, checksum := "chk.bin_format", "chk.bin_checksum"
	if iin {
		format, checksum = "chk.iin_format", "chk.iin_checksum"
	}
	if len(digits) != 12 || strings.IndexFunc(digits, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
		return bad(digits, format)
	}
	var d [12]int
	for i := range d {
		d[i] = int(digits[i] - '0')
	}
	if iin {
		century := map[int]int{1: 1800, 2: 1800, 3: 1900, 4: 1900, 5: 2000, 6: 2000}
		if c, ok := century[d[6]]; ok {
			if _, ok := validDate(c+d[0]*10+d[1], d[2]*10+d[3], d[4]*10+d[5]); !ok {
				return bad(digits, format)
			}
		} else if d[6] != 0 {
			return bad(digits, format)
		}
	}
	if !kzChecksum(d) {
		return bad(digits, checksum)
	}
	return Result{Value: digits, Typed: digits}
}

func kzChecksum(d [12]int) bool {
	sum := func(w []int) int {
		s := 0
		for i, x := range w {
			s += d[i] * x
		}
		return s % 11
	}
	s := sum([]int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11})
	if s == 10 {
		s = sum([]int{3, 4, 5, 6, 7, 8, 9, 10, 11, 1, 2})
		if s == 10 {
			return false
		}
	}
	return s == d[11]
}

func checkPhone(raw string) Result {
	var digits []byte
	for _, r := range raw {
		if r >= '0' && r <= '9' {
			digits = append(digits, byte(r))
		}
	}
	s := string(digits)
	switch {
	case len(s) == 11 && (s[0] == '8' || s[0] == '7'):
		s = "7" + s[1:]
	case len(s) == 10:
		s = "7" + s
	default:
		return bad(raw, "chk.phone")
	}
	v := fmt.Sprintf("+7 %s %s %s %s", s[1:4], s[4:7], s[7:9], s[9:11])
	return Result{Value: v, Typed: v}
}

// fold приводит вариант к виду для сравнения: регистр и кириллические двойники латиницы.
func fold(s string) string {
	lat := cyrToLat()
	return strings.Map(func(r rune) rune {
		if to, ok := lat[r]; ok {
			return to
		}
		return r
	}, strings.ToUpper(strings.TrimSpace(s)))
}

// cyrToLat — кириллические буквы, похожие на латинские.
func cyrToLat() map[rune]rune {
	return map[rune]rune{'А': 'A', 'В': 'B', 'Е': 'E', 'К': 'K', 'М': 'M', 'Н': 'H', 'О': 'O', 'Р': 'P', 'С': 'C', 'Т': 'T', 'Х': 'X', 'У': 'Y', 'І': 'I'}
}

func checkChoice(raw string, options []string) Result {
	var picked []string
	for _, part := range strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == ';' || r == '/' }) {
		if strings.TrimSpace(part) == "" {
			continue
		}
		found := ""
		for _, o := range options {
			if fold(o) == fold(part) {
				found = o
				break
			}
		}
		if found == "" {
			return bad(raw, "chk.choice")
		}
		picked = append(picked, found)
	}
	v := strings.Join(picked, ", ")
	return Result{Value: v, Typed: v}
}

func mixedScript(s string) bool {
	for _, word := range strings.FieldsFunc(s, func(r rune) bool { return !unicode.IsLetter(r) }) {
		cyr, lat := false, false
		for _, r := range word {
			cyr = cyr || unicode.Is(unicode.Cyrillic, r)
			lat = lat || unicode.Is(unicode.Latin, r)
		}
		if cyr && lat {
			return true
		}
	}
	return false
}

// Rows применяет Check ко всем ячейкам строк по колонкам шаблона (сохраняя уверенность модели).
func Rows(tpl tables.Template, rows []tables.Row) []tables.Row {
	out := make([]tables.Row, len(rows))
	for i, row := range rows {
		cells := make(map[string]tables.Cell, len(tpl.Columns))
		for _, col := range tpl.Columns {
			c := row.Cells[col.Name]
			res := Check(col, c.Value)
			c.Value, c.Typed, c.Issues = res.Value, res.Typed, res.Issues
			if c.Confidence == "" {
				c.Confidence = "high"
			}
			cells[col.Name] = c
		}
		out[i] = tables.Row{File: row.File, Page: row.Page, Cells: cells}
	}
	return out
}
