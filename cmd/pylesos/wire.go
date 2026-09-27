// Файл: cmd/pylesos/wire.go
package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/jmoiron/sqlx"

	"pylesos/internal/analyze"
	"pylesos/internal/api"
	docrepo "pylesos/internal/docforms/repository"
	docservice "pylesos/internal/docforms/service"
	"pylesos/internal/events"
	jobsrepo "pylesos/internal/jobs/repository"
	jobsservice "pylesos/internal/jobs/service"
	"pylesos/internal/normalize"
	"pylesos/internal/ollama"
	"pylesos/internal/paths"
	prepo "pylesos/internal/people/repository"
	pservice "pylesos/internal/people/service"
	"pylesos/internal/screens"
	settingsdomain "pylesos/internal/settings/domain"
	settingsrepo "pylesos/internal/settings/repository"
	settingsservice "pylesos/internal/settings/service"
	trepo "pylesos/internal/table/repository"
	tservice "pylesos/internal/table/service"
	tgservice "pylesos/internal/telegram/service"
	"pylesos/web"
)

type app struct {
	server *api.Server
	runner *jobsservice.Runner
	bot    *tgservice.Bot
}

// wire собирает зависимости; исполнители заданий регистрируются в Runner.
func wire(ctx context.Context, db *sqlx.DB, p paths.Paths, log *slog.Logger) (app, error) {
	bus := events.NewBus()
	defaults := settingsdomain.Settings{ExportsDir: p.ExportsDir, PhonePauseSec: settingsdomain.DefaultPhonePauseSec}
	settings := settingsservice.NewService(settingsrepo.NewSQLite(db), defaults)
	tg, err := newTelegram(ctx, db, p, log)
	if err != nil {
		return app{}, err
	}
	jobs := jobsrepo.NewSQLite(db)
	pages := normalize.New() // один движок PDF на программу (05-recognition-llm)
	forms := docservice.New(docrepo.NewSQLite(db), pages, time.Now)
	llm := ollama.New(ollama.DefaultURL)
	people := prepo.NewSQLite(db)
	analyzer := analyze.NewWorker(analyze.Deps{Gateway: tg.gateway, Messages: tg.msgs, People: people, Sources: tg.sources,
		Pages: pages, Forms: forms, LLM: llm, Models: analyze.OllamaPicker{Ollama: llm}, Now: time.Now})
	tableStore := trepo.NewSQLite(db)
	tableCfg := tservice.NewConfig(tableStore)
	exporter := tservice.NewExporter(tservice.ExportDeps{Config: tableCfg, Store: tableStore, Mentions: people, Sources: tg.sources,
		Settings: settings, LLM: llm, Models: analyze.OllamaPicker{Ollama: llm}, Write: writeExcel, Loc: time.Local})
	runner := jobsservice.NewRunner(ctx, jobs, bus, log, time.Now, tg.worker, analyzer, exporter)
	if err := runner.ResumeInterrupted(ctx); err != nil {
		return app{}, err
	}
	groups := tgservice.NewGroups(tg.gateway, tg.sources, runner, time.Now)
	login := tgservice.NewLogin(tg.gateway, tg.keys, groups)
	go syncAtStart(ctx, groups, log)
	bot := newBot(ctx, db, tg, bus, log)
	static, err := web.Dist()
	if err != nil {
		return app{}, err
	}
	builder := screens.NewBuilder(screens.Deps{Jobs: jobs, Kinds: runner, Settings: settings, Sources: tg.sources,
		Counts: tg.msgs, Stats: jobs, Login: login, Groups: groups, Bot: bot, Forms: forms,
		Ollama: llm, People: peopleStore{SQLite: people, svc: pservice.New(people, time.Now)}, Table: tableCfg, Loc: time.Local})
	server, err := api.New(api.Deps{Static: static, Bus: bus, Screens: builder, Jobs: runner, Sources: sourceActions{Groups: groups, runner: runner},
		Samples: forms, Log: log})
	if err != nil {
		return app{}, err
	}
	return app{server: server, runner: runner, bot: bot}, nil
}

// syncAtStart догружает группы при запуске (спецификация, раздел 3, п. 4); сеть не задерживает окно.
func syncAtStart(ctx context.Context, groups *tgservice.Groups, log *slog.Logger) {
	if err := groups.SyncAll(ctx); err != nil && ctx.Err() == nil {
		log.Warn("догрузка групп при запуске", "err", err)
	}
}
