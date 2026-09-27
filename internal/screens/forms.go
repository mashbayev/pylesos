// Файл: internal/screens/forms.go
package screens

import (
	"context"
	"errors"
	"fmt"

	docdomain "pylesos/internal/docforms/domain"
	"pylesos/internal/normalize"
	"pylesos/internal/tables"
	"pylesos/internal/texts"
)

// Forms — экран форм; selected > 0 — открыть редактор этой формы.
func (b *Builder) Forms(ctx context.Context, selected int64) (FormsScreen, error) {
	return b.forms(ctx, selected, "")
}

func (b *Builder) FormsCreate(ctx context.Context, name, fileName string, data []byte) (FormsScreen, error) {
	id, err := b.d.Forms.Create(ctx, name, fileName, data)
	return b.afterForm(ctx, id, err, texts.FormsCreated)
}

func (b *Builder) FormsSetZone(ctx context.Context, id int64, key string, z tables.Zone) (FormsScreen, error) {
	return b.afterForm(ctx, id, b.d.Forms.SetZone(ctx, id, key, z), texts.FormsSaved)
}

func (b *Builder) FormsClearZone(ctx context.Context, id int64, key string) (FormsScreen, error) {
	return b.afterForm(ctx, id, b.d.Forms.ClearZone(ctx, id, key), texts.FormsSaved)
}

func (b *Builder) FormsRename(ctx context.Context, id int64, name string) (FormsScreen, error) {
	return b.afterForm(ctx, id, b.d.Forms.Rename(ctx, id, name), texts.FormsSaved)
}

func (b *Builder) FormsDelete(ctx context.Context, id int64) (FormsScreen, error) {
	if err := b.d.Forms.Delete(ctx, id); err != nil {
		return b.afterForm(ctx, id, err, "")
	}
	return b.forms(ctx, 0, texts.FormsDeleted)
}

// afterForm — экран после действия: ошибка, которую исправит пользователь, — сообщением, прочие — ошибкой.
func (b *Builder) afterForm(ctx context.Context, id int64, err error, done string) (FormsScreen, error) {
	if err == nil {
		return b.forms(ctx, id, done)
	}
	if text := formProblem(err); text != "" {
		return b.forms(ctx, id, text)
	}
	return FormsScreen{}, fmt.Errorf("формы документов: %w", err)
}

func formProblem(err error) string {
	switch {
	case errors.Is(err, docdomain.ErrNoName):
		return texts.ErrFormsNoName
	case errors.Is(err, normalize.ErrPDFProtected):
		return texts.ErrFormsPDFProtected
	case errors.Is(err, normalize.ErrUnsupported):
		return texts.ErrFormsUnsupported
	case errors.Is(err, docdomain.ErrNoSample):
		return texts.ErrFormsNoSample
	case errors.Is(err, docdomain.ErrBadZone):
		return texts.ErrFormsBadZone
	case errors.Is(err, docdomain.ErrBadField):
		return texts.ErrFormsBadField
	case errors.Is(err, docdomain.ErrNotFound):
		return texts.ErrFormsNotFound
	}
	return ""
}

func (b *Builder) forms(ctx context.Context, selected int64, message string) (FormsScreen, error) {
	list, err := b.d.Forms.List(ctx)
	if err != nil {
		return FormsScreen{}, fmt.Errorf("формы документов: %w", err)
	}
	s := FormsScreen{Title: texts.FormsTitle, Hint: texts.FormsHint, NameLabel: texts.FormsName, FileLabel: texts.FormsFile,
		CreateLabel: texts.FormsCreate, SavingLabel: texts.Saving, Message: message, Forms: []FormRow{}}
	if len(list) == 0 {
		s.EmptyText = texts.FormsEmpty
	}
	for _, f := range list {
		s.Forms = append(s.Forms, FormRow{ID: f.ID, Name: f.Name, Status: formStatus(f), Selected: f.ID == selected, OpenLabel: texts.FormsOpen})
		if f.ID == selected {
			s.Editor = formEditor(f)
		}
	}
	return s, nil
}

func formStatus(f docdomain.Form) string {
	frame := texts.FormsFrameNo
	if f.Frame != nil {
		frame = texts.FormsFrameYes
	}
	return fmt.Sprintf(texts.FormsStatus, len(f.Zones), len(docdomain.Fields()), frame)
}

// zoneColor — постоянный цвет отметки: рамка и каждое поле своим цветом.
func zoneColor(i int) string { return fmt.Sprintf("hsl(%d 80%% 45%%)", (i*67)%360) }

func formEditor(f docdomain.Form) *FormEditor {
	e := &FormEditor{ID: f.ID, Name: f.Name, SampleURL: fmt.Sprintf("/api/forms/%d/sample", f.ID), Hint: texts.FormsEditorHint,
		ClearLabel: texts.FormsClear, RenameLabel: texts.FormsRename, DeleteLabel: texts.FormsDelete,
		Targets: []ZoneTarget{}, Boxes: []ZoneBox{}}
	keys := append([]string{docdomain.FrameKey}, docdomain.Fields()...)
	for i, key := range keys {
		label, zone, marked := texts.FormsFrame, f.Frame, f.Frame != nil
		if key != docdomain.FrameKey {
			z, ok := f.Zones[key]
			label, zone, marked = docdomain.FieldLabel(key), &z, ok
		}
		e.Targets = append(e.Targets, ZoneTarget{Key: key, Label: label, Marked: marked, Color: zoneColor(i)})
		if marked {
			e.Boxes = append(e.Boxes, ZoneBox{Key: key, Label: label, X: zone.X, Y: zone.Y, W: zone.W, H: zone.H, Color: zoneColor(i)})
		}
	}
	return e
}
