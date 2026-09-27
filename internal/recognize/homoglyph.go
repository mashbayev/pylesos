// Файл: internal/recognize/homoglyph.go
package recognize

import (
	"strings"
	"unicode"
)

// homoglyphs — латинские и греческие буквы, неотличимые на глаз от кириллических
// (включая казахские і, һ, ү). Модель иногда вставляет их в кириллические слова.
func homoglyphs() map[rune]rune {
	return map[rune]rune{
		'A': 'А', 'a': 'а', 'B': 'В', 'C': 'С', 'c': 'с', 'E': 'Е', 'e': 'е', 'H': 'Н', 'h': 'һ',
		'I': 'І', 'i': 'і', 'K': 'К', 'M': 'М', 'O': 'О', 'o': 'о', 'P': 'Р', 'p': 'р', 'T': 'Т',
		'X': 'Х', 'x': 'х', 'Y': 'Ү', 'y': 'у',
		'Α': 'А', 'α': 'а', 'Β': 'В', 'Ε': 'Е', 'Η': 'Н', 'Κ': 'К', 'Μ': 'М', 'Ο': 'О', 'ο': 'о',
		'Ρ': 'Р', 'ρ': 'р', 'Τ': 'Т', 'Χ': 'Х', 'χ': 'х',
	}
}

// fixHomoglyphs заменяет двойники на кириллицу только в словах, где уже есть кириллица;
// чисто латинские слова (адреса, названия) не меняются.
func fixHomoglyphs(s string) string {
	var b strings.Builder
	word := []rune{}
	glyphs := homoglyphs()
	flush := func() {
		cyrillic := false
		for _, r := range word {
			if unicode.Is(unicode.Cyrillic, r) {
				cyrillic = true
				break
			}
		}
		for _, r := range word {
			if to, ok := glyphs[r]; ok && cyrillic {
				r = to
			}
			b.WriteRune(r)
		}
		word = word[:0]
	}
	for _, r := range s {
		if unicode.IsLetter(r) {
			word = append(word, r)
			continue
		}
		flush()
		b.WriteRune(r)
	}
	flush()
	return b.String()
}
