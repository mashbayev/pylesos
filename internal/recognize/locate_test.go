// Файл: internal/recognize/locate_test.go
package recognize

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"math"
	"strings"
	"testing"

	"github.com/disintegration/imaging"

	"pylesos/internal/normalize"
	"pylesos/internal/tables"
)

// card — «удостоверение»: светлая карточка с тёмной фотографией и полосами текста.
func card() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 340, 215))
	draw.Draw(img, img.Bounds(), &image.Uniform{color.RGBA{225, 240, 230, 255}}, image.Point{}, draw.Src)
	draw.Draw(img, image.Rect(20, 60, 110, 180), &image.Uniform{color.RGBA{40, 40, 40, 255}}, image.Point{}, draw.Src)
	for i := 0; i < 5; i++ {
		draw.Draw(img, image.Rect(130, 70+i*22, 300-i*20, 80+i*22), &image.Uniform{color.RGBA{30, 30, 60, 255}}, image.Point{}, draw.Src)
	}
	draw.Draw(img, image.Rect(120, 10, 160, 50), &image.Uniform{color.RGBA{200, 160, 40, 255}}, image.Point{}, draw.Src)
	return img
}

func jpegOf(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// screenshot — светлый «чат» 590×1280 с карточкой нужного размера в заданном месте.
func screenshot(t *testing.T, width int, at image.Point, withCard bool) []byte {
	page := image.NewRGBA(image.Rect(0, 0, 590, 1280))
	draw.Draw(page, page.Bounds(), &image.Uniform{color.RGBA{245, 240, 232, 255}}, image.Point{}, draw.Src)
	draw.Draw(page, image.Rect(30, 100, 560, 160), &image.Uniform{color.RGBA{255, 255, 255, 255}}, image.Point{}, draw.Src)
	if withCard {
		scaled := imaging.Resize(card(), width, 0, imaging.Lanczos)
		draw.Draw(page, scaled.Bounds().Add(at), scaled, image.Point{}, draw.Src)
	}
	return jpegOf(t, page)
}

func TestLocateFindsCardAtAnotherScaleAndPlace(t *testing.T) {
	// Образец: скрин с карточкой шириной 340 в (100, 300); рамка — сама карточка.
	sample := screenshot(t, 340, image.Pt(100, 300), true)
	frame := tables.Zone{X: 100.0 / 590, Y: 300.0 / 1280, W: 340.0 / 590, H: 215.0 / 1280}
	loc, err := Locator(sample, frame)
	if err != nil || loc == "" {
		t.Fatalf("locator %q %v", loc, err)
	}
	page := screenshot(t, 280, image.Pt(180, 700), true) // другая страница: карточка меньше и ниже
	matches := Locate(loc, page)
	if len(matches) == 0 {
		t.Fatal("карточка не найдена")
	}
	m := matches[0]
	want := image.Rect(180, 700, 460, 877)
	if math.Abs(float64(m.Rect.Min.X-want.Min.X)) > 15 || math.Abs(float64(m.Rect.Min.Y-want.Min.Y)) > 15 ||
		math.Abs(float64(m.Rect.Dx()-want.Dx())) > 25 || m.Score < LocateThreshold {
		t.Fatalf("найдено %v (сходство %.2f), ожидалось около %v", m.Rect, m.Score, want)
	}
	if got := Locate(loc, screenshot(t, 280, image.Pt(180, 700), false)); len(got) != 0 {
		t.Fatalf("на скрине без карточки ничего не должно находиться: %+v", got)
	}
}

func TestZoneInFrameMapsToMatch(t *testing.T) {
	frame := tables.Zone{X: 0.2, Y: 0.3, W: 0.5, H: 0.2}
	zone := tables.Zone{X: 0.3, Y: 0.35, W: 0.1, H: 0.05} // в рамке: x 20%, y 25%, w 20%, h 25%
	got := zoneInMatch(zone, frame, image.Rect(100, 200, 300, 300), 600, 1000)
	want := tables.Zone{X: 140.0 / 600, Y: 225.0 / 1000, W: 40.0 / 600, H: 25.0 / 1000}
	if math.Abs(got.X-want.X) > 1e-9 || math.Abs(got.Y-want.Y) > 1e-9 || math.Abs(got.W-want.W) > 1e-9 || math.Abs(got.H-want.H) > 1e-9 {
		t.Fatalf("got %+v want %+v", got, want)
	}
}

func TestExtractPeopleModeReadsFoundCardsFirst(t *testing.T) {
	sample := screenshot(t, 340, image.Pt(100, 300), true)
	frame := tables.Zone{X: 100.0 / 590, Y: 300.0 / 1280, W: 340.0 / 590, H: 215.0 / 1280}
	loc, _ := Locator(sample, frame)
	tpl := tables.Template{Name: "ЧС", Mode: tables.ModePeople, Columns: []tables.Column{
		{Name: "ФИО", Type: tables.TypeText}, {Name: "Комментарий", Type: tables.TypeComment},
	}, Forms: []tables.Form{{Name: "Удостоверение", Frame: &frame, Locator: loc, Zones: map[string]tables.Zone{
		"ФИО": {X: 230.0 / 590, Y: 370.0 / 1280, W: 170.0 / 590, H: 12.0 / 1280},
	}}}}
	page := screenshot(t, 280, image.Pt(180, 700), true)
	q := &queueCore{replies: []string{
		`{"value":"ТЕСТОВ ИВАН","confidence":"high"}`,
		`{"rows":[{"ФИО":"ТЕСТОВ ИВАН","Комментарий":"Тестовый Автопарк: есть инфо?"}]}`,
	}}
	pages := []PageText{{Page: normalize.Page{Source: "скрин.jpg", Index: 1, Image: page}, Text: "Тестовый Автопарк ... есть инфо?"}}
	rows, err := (&Extractor{LLM: q, Model: "m", TextOnly: true}).Extract(context.Background(), tpl, "скрин.jpg", pages, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(q.reqs) != 2 || q.reqs[0].System != zonePrompt {
		t.Fatalf("сначала — чтение зоны найденной карточки: запросов %d", len(q.reqs))
	}
	if !strings.Contains(q.reqs[1].Text, "Документы, найденные по образцам") || !strings.Contains(q.reqs[1].Text, "ФИО: ТЕСТОВ ИВАН") {
		t.Fatalf("значения зон должны попасть в извлечение: %q", q.reqs[1].Text)
	}
	if len(rows) != 1 || rows[0].Cells["Комментарий"].Value != "Тестовый Автопарк: есть инфо?" {
		t.Fatalf("rows %+v", rows)
	}
}
