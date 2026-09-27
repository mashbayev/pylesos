// Файл: internal/docforms/service/service_test.go
package service

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"testing"
	"time"

	"pylesos/internal/docforms/domain"
	"pylesos/internal/docforms/repository"
	"pylesos/internal/normalize"
	"pylesos/internal/recognize"
	"pylesos/internal/store/storetest"
	"pylesos/internal/tables"
)

// samplePNG — вымышленный «документ»: полосы и прямоугольники, чтобы у образца был узнаваемый рисунок.
func samplePNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 400, 250))
	for y := 0; y < 250; y++ {
		for x := 0; x < 400; x++ {
			c := color.RGBA{R: uint8(x * 255 / 400), G: uint8(y * 255 / 250), B: 200, A: 255}
			if (x/40+y/25)%2 == 0 {
				c = color.RGBA{R: 30, G: 30, B: 30, A: 255}
			}
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func newService(t *testing.T) *Service {
	t.Helper()
	return New(repository.NewSQLite(storetest.Open(t)), normalize.New(), func() time.Time { return time.Unix(1000, 0) })
}

func TestCreateFromSample(t *testing.T) {
	ctx := context.Background()
	s := newService(t)
	if _, err := s.Create(ctx, "  ", "образец.png", samplePNG(t)); !errors.Is(err, domain.ErrNoName) {
		t.Fatalf("без названия: %v", err)
	}
	if _, err := s.Create(ctx, "Удостоверение", "образец.txt", []byte("x")); !errors.Is(err, domain.ErrNoSample) {
		t.Fatalf("не картинка: %v", err)
	}
	id, err := s.Create(ctx, " Удостоверение — лицевая ", "образец.png", samplePNG(t))
	if err != nil {
		t.Fatal(err)
	}
	f, err := s.Get(ctx, id)
	if err != nil || f.Name != "Удостоверение — лицевая" || len(f.Sample) == 0 || f.Signature == "" || len(f.Zones) != 0 {
		t.Fatalf("форма %+v %v", f.Name, err)
	}
}

func TestZoneValidation(t *testing.T) {
	ctx := context.Background()
	s := newService(t)
	id, err := s.Create(ctx, "Права", "образец.png", samplePNG(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetZone(ctx, id, domain.FieldIIN, tables.Zone{X: 0.9, Y: 0.1, W: 0.3, H: 0.1}); !errors.Is(err, domain.ErrBadZone) {
		t.Fatalf("зона за краем: %v", err)
	}
	if err := s.SetZone(ctx, id, "адрес", tables.Zone{X: 0.1, Y: 0.1, W: 0.2, H: 0.1}); !errors.Is(err, domain.ErrBadField) {
		t.Fatalf("неизвестное поле: %v", err)
	}
	if err := s.SetZone(ctx, id, domain.FieldIIN, tables.Zone{X: 0.1, Y: 0.1, W: 0.3, H: 0.1}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetZone(ctx, id, domain.FrameKey, tables.Zone{X: 0.05, Y: 0.05, W: 0.9, H: 0.9}); err != nil {
		t.Fatal(err)
	}
	f, err := s.Get(ctx, id)
	if err != nil || f.Zones[domain.FieldIIN].W != 0.3 || f.Frame == nil || f.Locator == "" {
		t.Fatalf("зоны не сохранились: %+v %v", f.Zones, err)
	}
	if err := s.ClearZone(ctx, id, domain.FrameKey); err != nil {
		t.Fatal(err)
	}
	if err := s.ClearZone(ctx, id, domain.FieldIIN); err != nil {
		t.Fatal(err)
	}
	if f, err := s.Get(ctx, id); err != nil || f.Frame != nil || f.Locator != "" || len(f.Zones) != 0 {
		t.Fatalf("после очистки: %+v %v", f.Zones, err)
	}
}

func TestAsTablesFormMatchesSample(t *testing.T) {
	ctx := context.Background()
	s := newService(t)
	id, err := s.Create(ctx, "Удостоверение", "образец.png", samplePNG(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetZone(ctx, id, domain.FieldFullName, tables.Zone{X: 0.1, Y: 0.2, W: 0.5, H: 0.1}); err != nil {
		t.Fatal(err)
	}
	f, err := s.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	tf := f.AsTablesForm()
	if _, ok := tf.Zones[domain.FieldLabel(domain.FieldFullName)]; !ok {
		t.Fatalf("зона ФИО под названием колонки: %+v", tf.Zones)
	}
	got, score := recognize.MatchForm([]tables.Form{tf}, f.Sample)
	if got == nil || score < recognize.MatchThreshold {
		t.Fatalf("образец не узнаёт сам себя: %v %.2f", got, score)
	}
	list, err := s.List(ctx)
	if err != nil || len(list) != 1 {
		t.Fatalf("список %d %v", len(list), err)
	}
	if err := s.Rename(ctx, id, "Удостоверение (лицевая)"); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(ctx, id); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(ctx, id); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("после удаления: %v", err)
	}
}

// Ревью M3, №2: причина нечитаемого образца не теряется (PDF с паролем — не «нужен PDF»).
func TestCreateKeepsSampleCause(t *testing.T) {
	data, err := os.ReadFile("../../normalize/testdata/protected.pdf")
	if err != nil {
		t.Fatal(err)
	}
	_, err = newService(t).Create(context.Background(), "Права", "образец.pdf", data)
	if !errors.Is(err, domain.ErrNoSample) || !errors.Is(err, normalize.ErrPDFProtected) {
		t.Fatalf("ждали ErrNoSample и ErrPDFProtected, получили %v", err)
	}
}
