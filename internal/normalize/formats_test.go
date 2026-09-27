// Файл: internal/normalize/formats_test.go
package normalize

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	"image/jpeg"
	"image/png"
	"path/filepath"
	"strings"
	"testing"
)

// buildTIFF — несжатый 8-битный серый TIFF (little endian) с несколькими страницами.
func buildTIFF(sizes [][2]int) []byte {
	var b bytes.Buffer
	le := binary.LittleEndian
	b.WriteString("II")
	binary.Write(&b, le, uint16(42))
	binary.Write(&b, le, uint32(0)) // смещение первой IFD — заполним позже
	prevNext := 4                   // куда писать смещение следующей IFD
	for _, sz := range sizes {
		w, h := sz[0], sz[1]
		dataOff := b.Len()
		for i := 0; i < w*h; i++ {
			v := byte(255)
			if i%w < w/2 {
				v = 0
			}
			b.WriteByte(v)
		}
		if b.Len()%2 == 1 {
			b.WriteByte(0)
		}
		ifdOff := b.Len()
		raw := b.Bytes()
		le.PutUint32(raw[prevNext:], uint32(ifdOff))
		entries := [][3]uint32{ // tag, type, value
			{256, 4, uint32(w)}, {257, 4, uint32(h)}, {258, 3, 8}, {259, 3, 1}, {262, 3, 1},
			{273, 4, uint32(dataOff)}, {277, 3, 1}, {278, 4, uint32(h)}, {279, 4, uint32(w * h)},
		}
		binary.Write(&b, le, uint16(len(entries)))
		for _, e := range entries {
			binary.Write(&b, le, uint16(e[0]))
			binary.Write(&b, le, uint16(e[1]))
			binary.Write(&b, le, uint32(1))
			if e[1] == 3 {
				binary.Write(&b, le, uint16(e[2]))
				binary.Write(&b, le, uint16(0))
			} else {
				binary.Write(&b, le, e[2])
			}
		}
		prevNext = b.Len()
		binary.Write(&b, le, uint32(0))
	}
	return b.Bytes()
}

func TestSupportedTIFFAndDOCX(t *testing.T) {
	for _, name := range []string{"a.tif", "b.TIFF", "c.docx"} {
		if !Supported(name) {
			t.Errorf("%s должен поддерживаться", name)
		}
	}
	if Supported("d.doc") {
		t.Error("старый .doc не поддерживается")
	}
}

func TestFileMultiPageTIFF(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "скан.tif"), buildTIFF([][2]int{{40, 30}, {3000, 20}, {10, 10}}))
	pages, err := File(root, "скан.tif")
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 3 {
		t.Fatalf("страниц %d", len(pages))
	}
	for i, p := range pages {
		if p.Index != i+1 || p.Source != "скан.tif" {
			t.Fatalf("страница %+v", p)
		}
	}
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(pages[1].Image))
	if err != nil || cfg.Width != MaxSide {
		t.Fatalf("вторая страница должна быть уменьшена до %d: %+v %v", MaxSide, cfg, err)
	}
}

func TestFileBrokenTIFF(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "bad.tiff"), []byte("II*\x00garbage"))
	if _, err := File(root, "bad.tiff"); !errors.Is(err, ErrBadImage) {
		t.Fatalf("ожидалась ErrBadImage, получено %v", err)
	}
}

const docxXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>
<w:p><w:r><w:t>ҚАЗАҚСТАН </w:t></w:r><w:r><w:t>РЕСПУБЛИКАСЫ</w:t></w:r></w:p>
<w:p><w:r><w:t>Анықтама</w:t><w:tab/><w:t>№ 12</w:t></w:r></w:p>
<w:tbl>
<w:tr><w:tc><w:p><w:r><w:t>ФИО</w:t></w:r></w:p></w:tc><w:tc><w:p><w:r><w:t>ИИН</w:t></w:r></w:p></w:tc></w:tr>
<w:tr><w:tc><w:p><w:r><w:t>Әбдіхалықов</w:t></w:r></w:p><w:p><w:r><w:t>Нұрсұлтан</w:t></w:r></w:p></w:tc><w:tc><w:p><w:r><w:t>850315|400124</w:t></w:r></w:p></w:tc></w:tr>
</w:tbl>
<w:p><w:r><w:t>Күні: 12.03.2024</w:t></w:r></w:p>
</w:body></w:document>`

func buildDOCX(t *testing.T, withImage bool) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("word/document.xml")
	w.Write([]byte(docxXML))
	if withImage {
		w, _ = zw.Create("word/media/image1.png")
		png.Encode(w, image.NewGray(image.Rect(0, 0, 20, 20)))
		w, _ = zw.Create("word/media/image2.emf")
		w.Write([]byte("векторная картинка не поддерживается"))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestFileDOCXTextTablesAndImages(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "анықтама.docx"), buildDOCX(t, true))
	pages, err := File(root, "анықтама.docx")
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 2 || pages[0].Image != nil || pages[1].Image == nil || pages[1].Index != 2 {
		t.Fatalf("страницы %+v", pages)
	}
	want := "ҚАЗАҚСТАН РЕСПУБЛИКАСЫ\nАнықтама\t№ 12\n\n| ФИО | ИИН |\n| --- | --- |\n| Әбдіхалықов Нұрсұлтан | 850315\\|400124 |\n\nКүні: 12.03.2024"
	if pages[0].TextLayer != want {
		t.Fatalf("текст\n%q\nwant\n%q", pages[0].TextLayer, want)
	}
}

func TestFileDOCXWithoutImages(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "a.docx"), buildDOCX(t, false))
	pages, err := File(root, "a.docx")
	if err != nil || len(pages) != 1 || !strings.Contains(pages[0].TextLayer, "РЕСПУБЛИКАСЫ") {
		t.Fatalf("pages %+v err %v", pages, err)
	}
}

func TestFileBrokenDOCX(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "bad.docx"), []byte("not a zip"))
	if _, err := File(root, "bad.docx"); !errors.Is(err, ErrBadDocument) {
		t.Fatalf("ожидалась ErrBadDocument, получено %v", err)
	}
}
