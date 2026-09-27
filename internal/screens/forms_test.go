// Файл: internal/screens/forms_test.go
package screens

import (
	"context"
	"fmt"
	"testing"
	"time"

	"pylesos/internal/docforms/domain"
	"pylesos/internal/normalize"
	"pylesos/internal/tables"
	"pylesos/internal/texts"
)

type fakeForms struct {
	forms   []domain.Form
	err     error
	created string
	zone    string
}

func (f *fakeForms) List(context.Context) ([]domain.Form, error) { return f.forms, nil }
func (f *fakeForms) Get(_ context.Context, id int64) (domain.Form, error) {
	for _, x := range f.forms {
		if x.ID == id {
			return x, nil
		}
	}
	return domain.Form{}, domain.ErrNotFound
}
func (f *fakeForms) Create(_ context.Context, name, _ string, _ []byte) (int64, error) {
	f.created = name
	return 1, f.err
}
func (f *fakeForms) SetZone(_ context.Context, _ int64, key string, _ tables.Zone) error {
	f.zone = key
	return f.err
}
func (f *fakeForms) ClearZone(context.Context, int64, string) error { return f.err }
func (f *fakeForms) Rename(context.Context, int64, string) error    { return f.err }
func (f *fakeForms) Delete(context.Context, int64) error            { return f.err }

func formsBuilder(f *fakeForms) *Builder {
	return NewBuilder(Deps{Jobs: fakeJobs{}, Kinds: fakeKinds{}, Settings: &fakeSettings{}, Sources: fakeSources{},
		Counts: fakeCounts{}, Stats: fakeStats{}, Login: &fakeLogin{}, Groups: &fakeGroups{}, Bot: &fakeBot{}, Forms: f, Ollama: fakeOllama{}, Loc: time.UTC})
}

func TestFormsListAndEditor(t *testing.T) {
	frame := tables.Zone{X: 0.1, Y: 0.1, W: 0.8, H: 0.8}
	f := &fakeForms{forms: []domain.Form{{ID: 1, Name: "Удостоверение", Frame: &frame,
		Zones: map[string]tables.Zone{domain.FieldIIN: {X: 0.2, Y: 0.3, W: 0.3, H: 0.05}}}}}
	s, err := formsBuilder(f).Forms(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if s.Title != texts.FormsTitle || len(s.Forms) != 1 || s.Editor != nil ||
		s.Forms[0].Status != fmt.Sprintf(texts.FormsStatus, 1, 4, texts.FormsFrameYes) {
		t.Fatalf("список: %+v", s)
	}
	s, err = formsBuilder(f).Forms(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	e := s.Editor
	if e == nil || e.SampleURL != "/api/forms/1/sample" || len(e.Targets) != 5 || e.Targets[0].Key != domain.FrameKey ||
		!e.Targets[0].Marked || e.Targets[2].Marked != true || len(e.Boxes) != 2 || e.Boxes[0].Color == "" {
		t.Fatalf("редактор: %+v", e)
	}
	if !s.Forms[0].Selected {
		t.Fatal("открытая форма не отмечена в списке")
	}
}

func TestFormsUserErrorsAsMessages(t *testing.T) {
	f := &fakeForms{err: domain.ErrNoSample}
	s, err := formsBuilder(f).FormsCreate(context.Background(), "Права", "a.txt", []byte("x"))
	if err != nil || s.Message != texts.ErrFormsNoSample || f.created != "Права" {
		t.Fatalf("ошибка образца: %+v %v", s.Message, err)
	}
	f.err = domain.ErrBadZone
	f.forms = []domain.Form{{ID: 1, Name: "Права", Zones: map[string]tables.Zone{}}}
	s, err = formsBuilder(f).FormsSetZone(context.Background(), 1, domain.FieldIIN, tables.Zone{})
	if err != nil || s.Message != texts.ErrFormsBadZone || s.Editor == nil {
		t.Fatalf("ошибка зоны: %+v %v", s.Message, err)
	}
	f.err = nil
	s, err = formsBuilder(f).FormsSetZone(context.Background(), 1, domain.FieldIIN, tables.Zone{X: 0.1, Y: 0.1, W: 0.1, H: 0.1})
	if err != nil || s.Message != texts.FormsSaved || f.zone != domain.FieldIIN {
		t.Fatalf("сохранение: %+v %v", s.Message, err)
	}
}

// Ревью M3, №2: экран называет настоящую причину — пароль или формат.
func TestFormsSampleCauseMessages(t *testing.T) {
	cases := map[error]string{
		fmt.Errorf("%w: %w", domain.ErrNoSample, normalize.ErrPDFProtected): texts.ErrFormsPDFProtected,
		fmt.Errorf("%w: %w", domain.ErrNoSample, normalize.ErrUnsupported):  texts.ErrFormsUnsupported,
		domain.ErrNoSample: texts.ErrFormsNoSample,
	}
	for err, want := range cases {
		s, gotErr := formsBuilder(&fakeForms{err: err}).FormsCreate(context.Background(), "Права", "a.pdf", nil)
		if gotErr != nil || s.Message != want {
			t.Fatalf("%v: %q %v", err, s.Message, gotErr)
		}
	}
}
