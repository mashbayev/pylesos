// Файл: internal/screens/table_test.go
package screens

import (
	"context"
	"testing"

	tdomain "pylesos/internal/table/domain"
	tservice "pylesos/internal/table/service"
	"pylesos/internal/tables"
	"pylesos/internal/texts"
)

type fakeTable struct{ cfg tdomain.Config }

func (f *fakeTable) Get(context.Context) (tdomain.Config, error) { return f.cfg, nil }
func (f *fakeTable) Save(_ context.Context, cfg tdomain.Config) error {
	if len(cfg.Columns) == 0 {
		return &tservice.ProblemError{Text: texts.ErrTableNoColumns}
	}
	f.cfg = cfg
	return nil
}

func TestTableScreenSaveAndProblem(t *testing.T) {
	f := &fakeTable{cfg: tdomain.Config{Columns: []tables.Column{{Name: "ИИН", Type: tables.TypeIIN}}}}
	b := NewBuilder(Deps{Table: f})
	s, err := b.Table(context.Background())
	if err != nil || len(s.Columns) != 1 || s.Columns[0].Type != "iin" || len(s.Types) != 5 || s.Types[3].Label != texts.TypeIIN {
		t.Fatalf("экран: %+v %v", s, err)
	}
	s, err = b.TableSave(context.Background(), []TableColumn{{Name: "Телефон", Type: "phone"}}, true)
	if err != nil || s.Message != texts.TableSaved || !s.IncludeQueries || f.cfg.Columns[0].Type != tables.TypePhone {
		t.Fatalf("сохранение: %+v %v", s, err)
	}
	if s, err = b.TableSave(context.Background(), nil, false); err != nil || s.Error != texts.ErrTableNoColumns {
		t.Fatalf("ошибка ввода: %+v %v", s, err)
	}
}
