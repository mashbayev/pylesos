// Файл: internal/paths/paths.go
// Package paths — где программа хранит данные и выгрузки (спецификация, раздел 1).
package paths

import (
	"path/filepath"
	"strings"
)

// AppDir — имя папки программы в папке данных и в «Документах».
const AppDir = "Пылесос"

// Paths — папки программы.
type Paths struct {
	DataDir    string // база, профиль браузера, временные файлы
	ExportsDir string // выгрузки Excel по умолчанию
}

// Resolve собирает пути из папки настроек ОС (os.UserConfigDir) и папки «Документы».
func Resolve(configDir, documentsDir string) Paths {
	return Paths{
		DataDir:    filepath.Join(configDir, AppDir),
		ExportsDir: filepath.Join(documentsDir, AppDir),
	}
}

// DocumentsDir — папка «Документы» пользователя. На Linux читается XDG_DOCUMENTS_DIR из
// ~/.config/user-dirs.dirs (там она может называться «Документы»), иначе — ~/Documents.
func DocumentsDir(goos, home string, readFile func(string) ([]byte, error)) string {
	fallback := filepath.Join(home, "Documents")
	if goos != "linux" {
		return fallback
	}
	data, err := readFile(filepath.Join(home, ".config", "user-dirs.dirs"))
	if err != nil {
		return fallback
	}
	for _, line := range strings.Split(string(data), "\n") {
		value, ok := strings.CutPrefix(strings.TrimSpace(line), "XDG_DOCUMENTS_DIR=")
		if !ok {
			continue
		}
		dir := filepath.Clean(strings.Replace(strings.Trim(value, `"`), "$HOME", home, 1))
		if dir == filepath.Clean(home) || !filepath.IsAbs(dir) {
			return fallback
		}
		return dir
	}
	return fallback
}
