// Файл: internal/normalize/normalize_test.go
package normalize

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writePNG(t *testing.T, path string, w, h int, c color.Color) {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, c)
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

func TestFileSourceUsesSlashes(t *testing.T) {
	root := t.TempDir()
	rel := filepath.Join("Сканы", "паспорт 1.png")
	writePNG(t, filepath.Join(root, rel), 10, 10, color.White)
	pages, err := File(root, rel)
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 1 || pages[0].Source != "Сканы/паспорт 1.png" || pages[0].Index != 1 {
		t.Fatalf("pages %+v", pages)
	}
	if _, err := jpeg.Decode(bytes.NewReader(pages[0].Image)); err != nil {
		t.Fatalf("Image не JPEG: %v", err)
	}
}

func TestFileDownscalesLargeImage(t *testing.T) {
	root := t.TempDir()
	writePNG(t, filepath.Join(root, "big.png"), 3000, 1000, color.White)
	pages, err := File(root, "big.png")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(pages[0].Image))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Width != MaxSide || cfg.Height != 800 {
		t.Fatalf("размер %dx%d", cfg.Width, cfg.Height)
	}
}

func TestFileFlattensTransparencyOnWhite(t *testing.T) {
	root := t.TempDir()
	writePNG(t, filepath.Join(root, "shot.png"), 8, 8, color.NRGBA{0, 0, 0, 0})
	pages, err := File(root, "shot.png")
	if err != nil {
		t.Fatal(err)
	}
	img, err := jpeg.Decode(bytes.NewReader(pages[0].Image))
	if err != nil {
		t.Fatal(err)
	}
	r, g, b, _ := img.At(4, 4).RGBA()
	if r>>8 < 245 || g>>8 < 245 || b>>8 < 245 {
		t.Fatalf("фон не белый: %d %d %d", r>>8, g>>8, b>>8)
	}
}

func TestFileCorruptImage(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "bad.jpg"), []byte("not an image"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := File(root, "bad.jpg"); !errors.Is(err, ErrBadImage) {
		t.Fatalf("ожидалась ErrBadImage, получено %v", err)
	}
}

func TestFileUnsupported(t *testing.T) {
	_, err := File(t.TempDir(), "a.heic")
	if !errors.Is(err, ErrUnsupported) || !strings.Contains(err.Error(), ".heic") {
		t.Fatalf("ожидалась ErrUnsupported с расширением, получено %v", err)
	}
}

// File — как в doc-agent: читает файл и приводит его к страницам (обёртка только для тестов).
func File(root, rel string) ([]Page, error) { return fileWith(New(), root, rel) }

func fileWith(n *Normalizer, root, rel string) ([]Page, error) {
	data, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	return n.Bytes(filepath.ToSlash(rel), data)
}
