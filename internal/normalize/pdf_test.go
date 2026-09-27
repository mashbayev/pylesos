// Файл: internal/normalize/pdf_test.go
package normalize

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// pdfPage — страница тестового PDF: картинка на всю страницу (скан) и/или текст (электронный PDF).
type pdfPage struct {
	jpeg []byte
	w, h int
	text string
}

// buildPDF собирает минимальный корректный PDF (A4, Helvetica, картинки через DCTDecode).
func buildPDF(pages []pdfPage) []byte {
	objs := []string{"", "", "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>"}
	add := func(body string) int { objs = append(objs, body); return len(objs) }
	var kids []string
	for _, p := range pages {
		res := "<< /Font << /F1 3 0 R >>"
		content := ""
		if p.jpeg != nil {
			img := add(fmt.Sprintf("<< /Type /XObject /Subtype /Image /Width %d /Height %d /ColorSpace /DeviceRGB /BitsPerComponent 8 /Filter /DCTDecode /Length %d >>\nstream\n%s\nendstream", p.w, p.h, len(p.jpeg), p.jpeg))
			res += fmt.Sprintf(" /XObject << /Im%d %d 0 R >>", img, img)
			content += fmt.Sprintf("q 595 0 0 842 0 0 cm /Im%d Do Q\n", img)
		}
		if p.text != "" {
			content += fmt.Sprintf("BT /F1 24 Tf 72 700 Td (%s) Tj ET\n", p.text)
		}
		res += " >>"
		c := add(fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content))
		pg := add(fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Resources %s /Contents %d 0 R >>", res, c))
		kids = append(kids, fmt.Sprintf("%d 0 R", pg))
	}
	objs[0] = "<< /Type /Catalog /Pages 2 0 R >>"
	objs[1] = fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), len(kids))
	var b bytes.Buffer
	b.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objs))
	for i, body := range objs {
		offsets[i] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, body)
	}
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(objs)+1)
	for _, o := range offsets {
		fmt.Fprintf(&b, "%010d 00000 n \n", o)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objs)+1, xref)
	return b.Bytes()
}

// scanJPEG — «скан»: белый лист с чёрным прямоугольником.
func scanJPEG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 200, 283))
	for y := 0; y < 283; y++ {
		for x := 0; x < 200; x++ {
			c := color.RGBA{255, 255, 255, 255}
			if x > 40 && x < 160 && y > 40 && y < 60 {
				c = color.RGBA{0, 0, 0, 255}
			}
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func writeFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSupportedPDF(t *testing.T) {
	if !Supported("скан.PDF") || !Supported("a.pdf") {
		t.Fatal("PDF должен поддерживаться")
	}
}

func testFilePDFScanPages(t *testing.T, n *Normalizer) {
	root := t.TempDir()
	j := scanJPEG(t)
	rel := filepath.Join("Құжаттар", "скан.pdf")
	writeFile(t, filepath.Join(root, rel), buildPDF([]pdfPage{{jpeg: j, w: 200, h: 283}, {jpeg: j, w: 200, h: 283}}))

	pages, err := fileWith(n, root, rel)
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 2 {
		t.Fatalf("страниц %d", len(pages))
	}
	for i, p := range pages {
		if p.Source != "Құжаттар/скан.pdf" || p.Index != i+1 || p.TextLayer != "" {
			t.Fatalf("страница %d: %+v", i+1, p)
		}
		cfg, err := jpeg.DecodeConfig(bytes.NewReader(p.Image))
		if err != nil {
			t.Fatalf("страница %d не JPEG: %v", i+1, err)
		}
		// A4 в 200 DPI: 595pt × 200/72 ≈ 1653 px.
		if cfg.Width < 1640 || cfg.Width > 1660 || cfg.Height < 2330 || cfg.Height > 2345 {
			t.Fatalf("размер %dx%d", cfg.Width, cfg.Height)
		}
	}
}

func testFilePDFTextLayer(t *testing.T, n *Normalizer) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "e.pdf"), buildPDF([]pdfPage{{text: "Invoice number 12345 total due"}}))
	pages, err := fileWith(n, root, "e.pdf")
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 1 || !strings.Contains(pages[0].TextLayer, "Invoice number 12345") || pages[0].Image == nil {
		t.Fatalf("pages %+v", pages)
	}
}

func testFilePDFShortTextIsNotALayer(t *testing.T, n *Normalizer) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "s.pdf"), buildPDF([]pdfPage{{jpeg: scanJPEG(t), w: 200, h: 283, text: "12"}}))
	pages, err := fileWith(n, root, "s.pdf")
	if err != nil {
		t.Fatal(err)
	}
	if pages[0].TextLayer != "" {
		t.Fatalf("случайные символы скана не должны считаться текстовым слоем: %q", pages[0].TextLayer)
	}
}

func testFileCorruptPDF(t *testing.T, n *Normalizer) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "bad.pdf"), []byte("%PDF-1.4\nэто не PDF"))
	if _, err := fileWith(n, root, "bad.pdf"); !errors.Is(err, ErrBadPDF) {
		t.Fatalf("ожидалась ErrBadPDF, получено %v", err)
	}
}

func testFileProtectedPDF(t *testing.T, n *Normalizer) {
	if _, err := fileWith(n, "testdata", "protected.pdf"); !errors.Is(err, ErrPDFProtected) {
		t.Fatalf("ожидалась ErrPDFProtected, получено %v", err)
	}
}

// TestPDF — один движок pdfium на все проверки PDF: запуск движка дорогой (особенно под -race).
func TestPDF(t *testing.T) {
	n := New()
	t.Run("FilePDFScanPages", func(t *testing.T) { testFilePDFScanPages(t, n) })
	t.Run("FilePDFTextLayer", func(t *testing.T) { testFilePDFTextLayer(t, n) })
	t.Run("FilePDFShortTextIsNotALayer", func(t *testing.T) { testFilePDFShortTextIsNotALayer(t, n) })
	t.Run("FileCorruptPDF", func(t *testing.T) { testFileCorruptPDF(t, n) })
	t.Run("FileProtectedPDF", func(t *testing.T) { testFileProtectedPDF(t, n) })
}
