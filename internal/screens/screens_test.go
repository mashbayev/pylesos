// Файл: internal/screens/screens_test.go
package screens

import (
	"context"
	"fmt"
	"testing"
	"time"

	jobsdomain "pylesos/internal/jobs/domain"
	settingsdomain "pylesos/internal/settings/domain"
	sourcesdomain "pylesos/internal/sources/domain"
	"pylesos/internal/texts"
)

type fakeJobs []jobsdomain.ListItem

func (f fakeJobs) List(context.Context) ([]jobsdomain.ListItem, error) { return f, nil }

type fakeKinds struct{}

func (fakeKinds) KindTitle(kind string) string { return "Вид " + kind }

type fakeSettings struct {
	current  settingsdomain.Settings
	problems map[string]string
}

func (f *fakeSettings) Get(context.Context) (settingsdomain.Settings, error) { return f.current, nil }
func (f *fakeSettings) Save(_ context.Context, in map[string]string) (settingsdomain.Settings, map[string]string, error) {
	if len(f.problems) > 0 {
		return f.current, f.problems, nil
	}
	f.current.ExportsDir = in[settingsdomain.KeyExportsDir]
	return f.current, nil, nil
}

func item(id int64, status jobsdomain.Status, reason string) jobsdomain.ListItem {
	return jobsdomain.ListItem{Job: jobsdomain.Job{ID: id, Kind: "x", Status: status, Reason: reason,
		ProgressText: "страница 2", UpdatedAt: time.Date(2026, 9, 26, 14, 5, 0, 0, time.UTC).Unix()}, SourceTitle: "Группа"}
}

type fakeSources []sourcesdomain.Source

func (f fakeSources) ListByKind(context.Context, sourcesdomain.Kind) ([]sourcesdomain.Source, error) {
	return f, nil
}

type fakeCounts map[int64]int

func (f fakeCounts) Counts(context.Context) (map[int64]int, error) { return f, nil }

type fakeStats map[int64]jobsdomain.SourceStats

func (f fakeStats) SourceStats(context.Context) (map[int64]jobsdomain.SourceStats, error) {
	return f, nil
}

func builder(jobs fakeJobs, s *fakeSettings) *Builder {
	return NewBuilder(Deps{Jobs: jobs, Kinds: fakeKinds{}, Settings: s, Sources: fakeSources{},
		Counts: fakeCounts{}, Stats: fakeStats{}, Login: &fakeLogin{}, Groups: &fakeGroups{}, Bot: &fakeBot{}, Forms: &fakeForms{}, Ollama: fakeOllama{}, Loc: time.UTC})
}

func TestJobsScreen(t *testing.T) {
	b := builder(fakeJobs{item(2, jobsdomain.StatusRunning, ""), item(1, jobsdomain.StatusDone, "")}, &fakeSettings{})
	s, err := b.Jobs(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	row := s.Jobs[0]
	if row.Title != "Вид x — Группа" || row.StatusText != texts.StatusRunning || row.UpdatedText != "26.09.2026 14:05" {
		t.Fatalf("строка: %+v", row)
	}
	if len(row.Actions) != 2 || row.Actions[0].Label != texts.ActionPause || row.Actions[1].Tone != "danger" {
		t.Fatalf("действия: %+v", row.Actions)
	}
	if s.Jobs[1].Actions == nil || len(s.Jobs[1].Actions) != 0 {
		t.Fatalf("у готового задания действий нет, и это [] а не nil: %#v", s.Jobs[1].Actions)
	}
	empty, err := builder(fakeJobs{}, &fakeSettings{}).Jobs(context.Background())
	if err != nil || empty.EmptyText != texts.JobsEmpty || empty.Jobs == nil {
		t.Fatalf("пустой экран: %+v %v", empty, err)
	}
}

func TestHomeBannersOnlyForWaiting(t *testing.T) {
	b := builder(fakeJobs{item(2, jobsdomain.StatusWaitingUser, "Войдите в OLX"), item(1, jobsdomain.StatusRunning, "")}, &fakeSettings{})
	s, err := b.Home(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := fmt.Sprintf(texts.BannerWaiting, "Группа", "Вид x", "Войдите в OLX")
	if len(s.Banners) != 1 || s.Banners[0].Text != want || s.Banners[0].JobID != 2 {
		t.Fatalf("плашки: %+v", s.Banners)
	}
}

func TestSettingsScreenShowsInputOnErrorsAndMessageOnSave(t *testing.T) {
	store := &fakeSettings{current: settingsdomain.Settings{ExportsDir: "/data", PhonePauseSec: 30},
		problems: map[string]string{settingsdomain.KeyExportsDir: texts.ErrExportsDirRelative}}
	b := builder(fakeJobs{}, store)
	in := map[string]string{settingsdomain.KeyExportsDir: "~/x", settingsdomain.KeyPhonePauseSec: "30"}
	s, err := b.SaveSettings(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if s.Fields[0].Value != "~/x" || s.Fields[0].Error != texts.ErrExportsDirRelative || s.Message != "" {
		t.Fatalf("экран с ошибкой: %+v", s)
	}
	store.problems = nil
	in[settingsdomain.KeyExportsDir] = "/new"
	s, err = b.SaveSettings(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if s.Fields[0].Value != "/new" || s.Fields[0].Error != "" || s.Message != texts.SettingsSaved || s.Fields[1].Value != "30" {
		t.Fatalf("экран после сохранения: %+v", s)
	}
}

// После «Выхода» окно показывает готовый текст от бэкенда, а не мёртвые кнопки.
func TestLayoutHasClosedText(t *testing.T) {
	if got := builder(fakeJobs{}, &fakeSettings{}).Layout().ClosedText; got != texts.ProgramClosed {
		t.Fatalf("closed_text %q", got)
	}
}
