// Файл: internal/normalize/formats.go
package normalize

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	"path"
	"sort"
	"strings"

	_ "golang.org/x/image/bmp"
	"golang.org/x/image/tiff"
)

var ErrBadDocument = errors.New("не удалось прочитать документ Word")

// tiffPages декодирует все страницы TIFF: для каждой IFD из цепочки подменяется смещение
// первой IFD в заголовке, и стандартный декодер читает нужную страницу.
func tiffPages(data []byte, source string) ([]Page, error) {
	bo, offsets, err := tiffOffsets(data)
	if err != nil {
		return nil, err
	}
	pages := make([]Page, 0, len(offsets))
	buf := make([]byte, len(data))
	for i, off := range offsets {
		copy(buf, data)
		bo.PutUint32(buf[4:8], off)
		img, err := tiff.Decode(bytes.NewReader(buf))
		if err != nil {
			return nil, fmt.Errorf("%w: страница %d: %v", ErrBadImage, i+1, err)
		}
		jpg, err := prepareImage(img)
		if err != nil {
			return nil, err
		}
		pages = append(pages, Page{Source: source, Index: i + 1, Image: jpg})
	}
	return pages, nil
}

// tiffOffsets — порядок байт и смещения всех страниц (IFD) TIFF.
func tiffOffsets(data []byte) (binary.ByteOrder, []uint32, error) {
	if len(data) < 8 {
		return nil, nil, fmt.Errorf("%w: TIFF слишком короткий", ErrBadImage)
	}
	var bo binary.ByteOrder
	switch string(data[:2]) {
	case "II":
		bo = binary.LittleEndian
	case "MM":
		bo = binary.BigEndian
	default:
		return nil, nil, fmt.Errorf("%w: не TIFF", ErrBadImage)
	}
	if bo.Uint16(data[2:4]) != 42 {
		return nil, nil, fmt.Errorf("%w: неподдерживаемый вариант TIFF", ErrBadImage)
	}
	var offsets []uint32
	seen := map[uint32]bool{}
	for off := bo.Uint32(data[4:8]); off != 0 && !seen[off] && len(offsets) < 1000; {
		seen[off] = true
		if int(off)+2 > len(data) {
			break
		}
		next := int(off) + 2 + int(bo.Uint16(data[off:]))*12
		if next+4 > len(data) {
			break
		}
		offsets = append(offsets, off)
		off = bo.Uint32(data[next:])
	}
	if len(offsets) == 0 {
		return nil, nil, fmt.Errorf("%w: в TIFF нет страниц", ErrBadImage)
	}
	return bo, offsets, nil
}

func isDocxImage(name string) bool {
	switch strings.ToLower(path.Ext(name)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".bmp", ".tif", ".tiff":
		return true
	}
	return false
}

// docxPages: страница 1 — текст документа (таблицы в Markdown), далее — встроенные картинки.
func docxPages(data []byte, source string) ([]Page, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBadDocument, err)
	}
	text, media, err := docxParts(zr)
	if err != nil {
		return nil, err
	}
	var pages []Page
	if text != "" {
		pages = append(pages, Page{Source: source, Index: 1, TextLayer: text})
	}
	for _, f := range media {
		jpg, ok := docxImage(f)
		if !ok {
			continue // повреждённая или неподдерживаемая картинка не мешает тексту
		}
		pages = append(pages, Page{Source: source, Index: len(pages) + 1, Image: jpg})
	}
	if len(pages) == 0 {
		return nil, fmt.Errorf("%w: в документе нет текста и картинок", ErrBadDocument)
	}
	return pages, nil
}

// docxParts — текст документа и файлы встроенных картинок по порядку.
func docxParts(zr *zip.Reader) (string, []*zip.File, error) {
	var text string
	var media []*zip.File
	for _, f := range zr.File {
		switch {
		case f.Name == "word/document.xml":
			rc, err := f.Open()
			if err != nil {
				return "", nil, fmt.Errorf("%w: %v", ErrBadDocument, err)
			}
			text, err = docxText(rc)
			if err = errors.Join(err, rc.Close()); err != nil {
				return "", nil, fmt.Errorf("%w: %v", ErrBadDocument, err)
			}
		case strings.HasPrefix(f.Name, "word/media/") && isDocxImage(f.Name):
			media = append(media, f)
		}
	}
	sort.Slice(media, func(i, j int) bool { return media[i].Name < media[j].Name })
	return text, media, nil
}

// docxImage — картинка из документа в JPEG; false — не читается (такую картинку пропускаем).
func docxImage(f *zip.File) ([]byte, bool) {
	rc, err := f.Open()
	if err != nil {
		return nil, false
	}
	img, _, decodeErr := image.Decode(rc)
	if err := errors.Join(decodeErr, rc.Close()); err != nil {
		return nil, false
	}
	jpg, err := prepareImage(img)
	return jpg, err == nil
}
