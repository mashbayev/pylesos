// Файл: internal/normalize/pdf.go
package normalize

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/klippa-app/go-pdfium"
	pdfiumerrors "github.com/klippa-app/go-pdfium/errors"
	"github.com/klippa-app/go-pdfium/references"
	"github.com/klippa-app/go-pdfium/requests"
	"github.com/klippa-app/go-pdfium/webassembly"
)

var (
	ErrBadPDF       = errors.New("не удалось открыть PDF")
	ErrPDFProtected = errors.New("PDF защищён паролем")
)

const (
	pdfDPI = 200
	// minTextLetters — сколько букв/цифр нужно, чтобы считать текст страницы текстовым слоем,
	// а не случайными символами из скана.
	minTextLetters = 20
)

func (n *Normalizer) pdfEngine() (pdfium.Pdfium, error) {
	n.pdfOnce.Do(func() {
		pool, err := webassembly.Init(webassembly.Config{MinIdle: 1, MaxIdle: 1, MaxTotal: 1})
		if err != nil {
			n.pdfErr = err
			return
		}
		n.pdf, n.pdfErr = pool.GetInstance(30 * time.Second)
	})
	return n.pdf, n.pdfErr
}

// pdfPages рендерит каждую страницу в pdfDPI и забирает текстовый слой, если он есть.
func (n *Normalizer) pdfPages(data []byte, source string) ([]Page, error) {
	engine, err := n.pdfEngine()
	if err != nil {
		return nil, fmt.Errorf("движок PDF не запустился: %w", err)
	}
	n.pdfMu.Lock()
	defer n.pdfMu.Unlock()
	doc, err := engine.OpenDocument(&requests.OpenDocument{File: &data})
	if errors.Is(err, pdfiumerrors.ErrPassword) {
		return nil, ErrPDFProtected
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBadPDF, err)
	}
	pages, err := renderPages(engine, doc.Document, source)
	if _, closeErr := engine.FPDF_CloseDocument(&requests.FPDF_CloseDocument{Document: doc.Document}); closeErr != nil && err == nil {
		err = fmt.Errorf("%w: закрыть документ: %v", ErrBadPDF, closeErr)
	}
	return pages, err
}

func renderPages(engine pdfium.Pdfium, doc references.FPDF_DOCUMENT, source string) ([]Page, error) {
	count, err := engine.FPDF_GetPageCount(&requests.FPDF_GetPageCount{Document: doc})
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBadPDF, err)
	}
	if count.PageCount == 0 {
		return nil, fmt.Errorf("%w: в документе нет страниц", ErrBadPDF)
	}
	pages := make([]Page, 0, count.PageCount)
	for i := 0; i < count.PageCount; i++ {
		p, err := renderPage(engine, doc, i)
		if err != nil {
			return nil, err
		}
		p.Source = source
		pages = append(pages, p)
	}
	return pages, nil
}

func renderPage(engine pdfium.Pdfium, doc references.FPDF_DOCUMENT, i int) (Page, error) {
	page := requests.Page{ByIndex: &requests.PageByIndex{Document: doc, Index: i}}
	render, err := engine.RenderPageInDPI(&requests.RenderPageInDPI{Page: page, DPI: pdfDPI})
	if err != nil {
		return Page{}, fmt.Errorf("%w: страница %d: %v", ErrBadPDF, i+1, err)
	}
	// Буфер пикселей живёт до Cleanup — кодируем сразу.
	img, err := prepareImage(render.Result.RenderedImage)
	render.Cleanup()
	if err != nil {
		return Page{}, fmt.Errorf("%w: страница %d: %v", ErrBadPDF, i+1, err)
	}
	layer := ""
	// Текстовый слой необязателен: нет его или не читается — страница остаётся картинкой.
	if text, err := engine.GetPageText(&requests.GetPageText{Page: page}); err == nil && letters(text.Text) >= minTextLetters {
		layer = strings.TrimSpace(text.Text)
	}
	return Page{Index: i + 1, Image: img, TextLayer: layer}, nil
}

func letters(s string) int {
	n := 0
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			n++
		}
	}
	return n
}
