// Файл: internal/paths/paths_test.go
package paths

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestResolve(t *testing.T) {
	p := Resolve("/cfg", "/home/u/Документы")
	if p.DataDir != filepath.Join("/cfg", "Пылесос") {
		t.Fatalf("DataDir = %q", p.DataDir)
	}
	if p.ExportsDir != filepath.Join("/home/u/Документы", "Пылесос") {
		t.Fatalf("ExportsDir = %q", p.ExportsDir)
	}
}

func TestDocumentsDir(t *testing.T) {
	noFile := func(string) ([]byte, error) { return nil, errors.New("нет файла") }
	file := func(body string) func(string) ([]byte, error) {
		return func(string) ([]byte, error) { return []byte(body), nil }
	}
	fallback := filepath.Join("/home/u", "Documents")
	cases := []struct {
		name, goos string
		read       func(string) ([]byte, error)
		want       string
	}{
		{"windows", "windows", noFile, fallback},
		{"linux без user-dirs", "linux", noFile, fallback},
		{"linux русская папка", "linux", file("# коммент\nXDG_DOCUMENTS_DIR=\"$HOME/Документы\"\n"), "/home/u/Документы"},
		{"linux папка равна домашней", "linux", file("XDG_DOCUMENTS_DIR=\"$HOME/\"\n"), fallback},
		{"linux нет ключа", "linux", file("XDG_MUSIC_DIR=\"$HOME/Music\"\n"), fallback},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := DocumentsDir(c.goos, "/home/u", c.read); got != c.want {
				t.Fatalf("got %q, want %q", got, c.want)
			}
		})
	}
}
