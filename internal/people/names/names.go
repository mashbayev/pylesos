// Файл: internal/people/names/names.go
// Package names — нормализация ФИО и похожесть имён для склейки людей (П17; спецификация, раздел 4, шаг 4).
package names

import (
	"slices"
	"strings"
	"unicode"
)

// Key — нормализованное ФИО: нижний регистр, ё→е, казахские буквы → русские, латиница → кириллица,
// слова по алфавиту (порядок «Фамилия Имя» не важен). Пустая строка — имени нет.
func Key(name string) string {
	s := strings.ToLower(name)
	s = strings.NewReplacer(kazakhPairs()...).Replace(s)
	s = translit(s)
	words := strings.FieldsFunc(s, func(r rune) bool { return !unicode.IsLetter(r) })
	words = slices.DeleteFunc(words, isMarker)
	slices.Sort(words)
	return strings.Join(words, " ")
}

// isMarker — служебные слова групп-чёрных списков, а не часть ФИО.
func isMarker(w string) bool {
	switch w {
	case "чс", "черный", "список":
		return true
	}
	return false
}

func kazakhPairs() []string {
	return []string{"ё", "е", "ә", "а", "ғ", "г", "қ", "к", "ң", "н", "ө", "о", "ұ", "у", "ү", "у", "һ", "х", "і", "и"}
}

// translit — латиница → кириллица: сначала сочетания, затем буквы.
func translit(s string) string {
	s = strings.NewReplacer("shch", "щ", "sh", "ш", "ch", "ч", "zh", "ж", "kh", "х", "ts", "ц",
		"yu", "ю", "ya", "я", "yo", "е", "ye", "е").Replace(s)
	letters := map[rune]string{'a': "а", 'b': "б", 'c': "к", 'd': "д", 'e': "е", 'f': "ф", 'g': "г", 'h': "х",
		'i': "и", 'j': "ж", 'k': "к", 'l': "л", 'm': "м", 'n': "н", 'o': "о", 'p': "п", 'q': "к", 'r': "р",
		's': "с", 't': "т", 'u': "у", 'v': "в", 'w': "в", 'x': "кс", 'y': "ы", 'z': "з"}
	var b strings.Builder
	for _, r := range s {
		if c, ok := letters[r]; ok {
			b.WriteString(c)
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// Similar — ключи разные, но, возможно, это один человек: у одного есть отчество
// (остальные слова совпадают, их не меньше двух), либо одно слово отличается одной буквой
// (в более длинном из двух слов не меньше 5 букв). При сомнении — false: ложная склейка хуже (П17).
func Similar(a, b string) bool {
	if a == b || a == "" || b == "" {
		return false
	}
	ra, rb := unmatched(strings.Fields(a), strings.Fields(b))
	common := len(strings.Fields(a)) - len(ra)
	switch {
	case len(ra) == 0 && len(rb) == 1, len(ra) == 1 && len(rb) == 0:
		return common >= 2
	case len(ra) == 1 && len(rb) == 1:
		longer := max(len([]rune(ra[0])), len([]rune(rb[0])))
		return longer >= 5 && distance(ra[0], rb[0]) <= 1
	}
	return false
}

// unmatched — слова каждого списка, не нашедшие пару в другом.
func unmatched(a, b []string) ([]string, []string) {
	left := slices.Clone(b)
	var ra []string
	for _, w := range a {
		if i := slices.Index(left, w); i >= 0 {
			left = slices.Delete(left, i, i+1)
			continue
		}
		ra = append(ra, w)
	}
	return ra, left
}

// distance — расстояние Левенштейна по буквам.
func distance(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur := make([]int, len(rb)+1)
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(rb)]
}
