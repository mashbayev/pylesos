// Файл: internal/normalize/normalize.go
// Package normalize приводит файлы (фото, PDF, TIFF, Word из Telegram) к списку страниц-изображений.
// Перенесено из doc-agent (П5): работа с байтами вместо папок, движок PDF — поле Normalizer, а не глобальное состояние.
package normalize

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"path/filepath"
	"strings"
	"sync"

	"github.com/disintegration/imaging"
	"github.com/klippa-app/go-pdfium"
)

// MaxSide — максимальная длина стороны изображения, отправляемого модели.
const MaxSide = 2400

var (
	ErrUnsupported = errors.New("неподдерживаемый формат")
	ErrBadImage    = errors.New("не удалось прочитать изображение")
)

type Page struct {
	Source    string // имя файла, со слешами "/"
	Index     int    // номер страницы с 1
	Image     []byte // JPEG; nil, если у страницы только текст
	TextLayer string
}

func isImageExt(ext string) bool { return ext == ".jpg" || ext == ".jpeg" || ext == ".png" }

func Supported(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	return isImageExt(ext) || ext == ".pdf" || ext == ".tif" || ext == ".tiff" || ext == ".docx"
}

// Normalizer — приведение файлов к страницам. Движок pdfium (WebAssembly) запускается один раз при первом PDF;
// экземпляр не потокобезопасен — запросы к нему идут под мьютексом.
type Normalizer struct {
	pdfOnce sync.Once
	pdf     pdfium.Pdfium
	pdfErr  error
	pdfMu   sync.Mutex
}

func New() *Normalizer { return &Normalizer{} }

// Bytes — страницы файла name (по расширению выбирается формат).
func (n *Normalizer) Bytes(name string, data []byte) ([]Page, error) {
	if !Supported(name) {
		return nil, fmt.Errorf("%w: %s", ErrUnsupported, filepath.Ext(name))
	}
	switch strings.ToLower(filepath.Ext(name)) {
	case ".pdf":
		return n.pdfPages(data, name)
	case ".tif", ".tiff":
		return tiffPages(data, name)
	case ".docx":
		return docxPages(data, name)
	}
	src, err := imaging.Decode(bytes.NewReader(data), imaging.AutoOrientation(true))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBadImage, err)
	}
	img, err := prepareImage(src)
	if err != nil {
		return nil, err
	}
	return []Page{{Source: name, Index: 1, Image: img}}, nil
}

// prepareImage кладёт изображение на белый фон, уменьшает до MaxSide и кодирует в JPEG.
func prepareImage(src image.Image) ([]byte, error) {
	b := src.Bounds()
	var img image.Image = imaging.Overlay(imaging.New(b.Dx(), b.Dy(), color.White), src, image.Pt(0, 0), 1.0)
	if b.Dx() > MaxSide || b.Dy() > MaxSide {
		img = imaging.Fit(img, MaxSide, MaxSide, imaging.Lanczos)
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 92}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
