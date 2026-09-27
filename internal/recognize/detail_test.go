// Файл: internal/recognize/detail_test.go
package recognize

import (
	"bytes"
	"context"
	"image"
	"image/jpeg"
	"strings"
	"testing"

	"pylesos/internal/normalize"
)

func blankJPEG(t *testing.T, w, h int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, image.NewRGBA(image.Rect(0, 0, w, h)), nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestDetailReadsOverlappingUpscaledTiles(t *testing.T) {
	q := &queueCore{replies: []string{"ОТЫНШИЕВ", "<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<", "060403962"}}
	page := normalize.Page{Source: "скрин.jpg", Index: 1, Image: blankJPEG(t, 600, 1300)}
	got, err := (&Transcriber{LLM: q, Model: "m"}).Detail(context.Background(), page)
	if err != nil {
		t.Fatal(err)
	}
	if len(q.reqs) != 3 {
		t.Fatalf("полос %d, ожидалось 3", len(q.reqs))
	}
	for i, r := range q.reqs {
		cfg, err := jpeg.DecodeConfig(bytes.NewReader(r.Images[0]))
		if err != nil || cfg.Width != 1200 {
			t.Fatalf("полоса %d: ширина %d (%v)", i+1, cfg.Width, err)
		}
		if r.System != transcribePrompt {
			t.Fatal("полосы читаются тем же строгим промптом")
		}
	}
	if !strings.Contains(got, "ОТЫНШИЕВ") || !strings.Contains(got, "060403962") || strings.Contains(got, "<<<<") {
		t.Fatalf("подробный текст %q", got)
	}
}

func TestDetailSkipsLargeScans(t *testing.T) {
	q := &queueCore{}
	page := normalize.Page{Source: "a.pdf", Index: 1, Image: blankJPEG(t, 1654, 2339)}
	got, err := (&Transcriber{LLM: q, Model: "m"}).Detail(context.Background(), page)
	if err != nil || got != "" || len(q.reqs) != 0 {
		t.Fatalf("крупной странице подробное чтение не нужно: %q %v %d", got, err, len(q.reqs))
	}
}
