// Файл: internal/settings/service/validate.go
package service

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"pylesos/internal/settings/domain"
	"pylesos/internal/texts"
)

// validate возвращает очищенные значения для записи и ошибки по ключам полей.
func validate(input map[string]string) (map[string]string, map[string]string) {
	values := map[string]string{}
	problems := map[string]string{}
	dir := strings.TrimSpace(input[domain.KeyExportsDir])
	switch {
	case dir == "":
		problems[domain.KeyExportsDir] = texts.ErrExportsDirEmpty
	case !filepath.IsAbs(dir):
		problems[domain.KeyExportsDir] = texts.ErrExportsDirRelative
	default:
		values[domain.KeyExportsDir] = filepath.Clean(dir)
	}
	pause, err := strconv.Atoi(strings.TrimSpace(input[domain.KeyPhonePauseSec]))
	switch {
	case err != nil:
		problems[domain.KeyPhonePauseSec] = texts.ErrPauseNotNumber
	case !inPauseRange(pause):
		problems[domain.KeyPhonePauseSec] = fmt.Sprintf(texts.ErrPauseRange, domain.MinPhonePauseSec, domain.MaxPhonePauseSec)
	default:
		values[domain.KeyPhonePauseSec] = strconv.Itoa(pause)
	}
	return values, problems
}

func inPauseRange(n int) bool {
	return n >= domain.MinPhonePauseSec && n <= domain.MaxPhonePauseSec
}
