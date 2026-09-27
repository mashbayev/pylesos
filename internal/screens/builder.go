// Файл: internal/screens/builder.go
package screens

import (
	"context"
	"time"

	docdomain "pylesos/internal/docforms/domain"
	jobsdomain "pylesos/internal/jobs/domain"
	settingsdomain "pylesos/internal/settings/domain"
	sourcesdomain "pylesos/internal/sources/domain"
	"pylesos/internal/tables"
	tgservice "pylesos/internal/telegram/service"
	"pylesos/internal/texts"
)

type JobLister interface {
	List(ctx context.Context) ([]jobsdomain.ListItem, error)
}

type KindTitler interface {
	KindTitle(kind string) string
}

type SettingsStore interface {
	Get(ctx context.Context) (settingsdomain.Settings, error)
	Save(ctx context.Context, input map[string]string) (settingsdomain.Settings, map[string]string, error)
}

type SourceLister interface {
	ListByKind(ctx context.Context, kind sourcesdomain.Kind) ([]sourcesdomain.Source, error)
}

type MessageCounter interface {
	Counts(ctx context.Context) (map[int64]int, error)
}

type JobStats interface {
	SourceStats(ctx context.Context) (map[int64]jobsdomain.SourceStats, error)
}

type TelegramLogin interface {
	State(ctx context.Context) (tgservice.LoginState, error)
	Submit(ctx context.Context, values map[string]string) (map[string]string, error)
	EditKeys()
}

type TelegramGroups interface {
	List(ctx context.Context, refresh bool) ([]tgservice.GroupView, error)
	Add(ctx context.Context, key string) error
}

type TelegramBot interface {
	Status() tgservice.BotStatus
	SetToken(ctx context.Context, token string) (string, error)
}

// FormStore — формы документов (docforms/service).
type FormStore interface {
	List(ctx context.Context) ([]docdomain.Form, error)
	Get(ctx context.Context, id int64) (docdomain.Form, error)
	Create(ctx context.Context, name, fileName string, data []byte) (int64, error)
	SetZone(ctx context.Context, id int64, key string, z tables.Zone) error
	ClearZone(ctx context.Context, id int64, key string) error
	Rename(ctx context.Context, id int64, name string) error
	Delete(ctx context.Context, id int64) error
}

// OllamaChecker — установленные модели Ollama, понимающие изображения.
type OllamaChecker interface {
	VisionModels(ctx context.Context) ([]string, error)
}

// Deps — откуда экраны берут данные.
type Deps struct {
	Jobs     JobLister
	Kinds    KindTitler
	Settings SettingsStore
	Sources  SourceLister
	Counts   MessageCounter
	Stats    JobStats
	Login    TelegramLogin
	Groups   TelegramGroups
	Bot      TelegramBot
	Forms    FormStore
	Ollama   OllamaChecker
	People   PeopleStore
	Table    TableConfig
	Loc      *time.Location
}

type Builder struct{ d Deps }

func NewBuilder(d Deps) *Builder { return &Builder{d: d} }

func (b *Builder) Layout() Layout {
	return Layout{
		AppName: texts.AppName,
		Nav: []NavItem{
			{ID: "home", Label: texts.NavHome},
			{ID: "telegram", Label: texts.NavTelegram},
			{ID: "people", Label: texts.NavPeople},
			{ID: "table", Label: texts.NavTable},
			{ID: "forms", Label: texts.NavForms},
			{ID: "jobs", Label: texts.NavJobs},
			{ID: "settings", Label: texts.NavSettings},
		},
		ThemeLabel: texts.ThemeToggle,
		QuitLabel:  texts.Quit,
		ClosedText: texts.ProgramClosed,
	}
}

// formatTime — время для экранов: «27.09.2026 09:30» в часовом поясе пользователя.
func (b *Builder) formatTime(unix int64) string {
	return time.Unix(unix, 0).In(b.d.Loc).Format("02.01.2006 15:04")
}
