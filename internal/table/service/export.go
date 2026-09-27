// Файл: internal/table/service/export.go
package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"pylesos/internal/analyze"
	jobsdomain "pylesos/internal/jobs/domain"
	jobsservice "pylesos/internal/jobs/service"
	"pylesos/internal/llm"
	"pylesos/internal/normalize"
	"pylesos/internal/ollama"
	pdomain "pylesos/internal/people/domain"
	"pylesos/internal/recognize"
	settingsdomain "pylesos/internal/settings/domain"
	sourcesdomain "pylesos/internal/sources/domain"
	"pylesos/internal/table/domain"
	"pylesos/internal/tables"
	tgdomain "pylesos/internal/telegram/domain"
	"pylesos/internal/texts"
	"pylesos/internal/validate"
)

// KindExport — вид задания «выгрузка людей группы в Excel».
const KindExport = "tg_export"

// Mentions — сообщения о человеке (people/repository).
type Mentions interface {
	PersonMentions(ctx context.Context, personID int64) ([]pdomain.MentionView, error)
}

type Sources interface {
	Get(ctx context.Context, id int64) (sourcesdomain.Source, error)
}

type Settings interface {
	Get(ctx context.Context) (settingsdomain.Settings, error)
}

// ModelPicker — какой моделью заполнять колонки.
type ModelPicker interface {
	Pick(ctx context.Context) (string, error)
}

// WriteExcel — запись книги (export.WriteExcel).
type WriteExcel func(outDir string, tpl tables.Template, rows []tables.Row) (string, error)

type ExportDeps struct {
	Config   *Config
	Store    Store
	Mentions Mentions
	Sources  Sources
	Settings Settings
	LLM      llm.Responder
	Models   ModelPicker
	Write    WriteExcel
	Loc      *time.Location
}

// Exporter — задание: по каждому человеку группы модель заполняет колонки из текста его документов,
// к строке добавляются комментарии из чата; готовые строки хранятся — повтор не гоняет модель.
type Exporter struct{ d ExportDeps }

func NewExporter(d ExportDeps) *Exporter { return &Exporter{d: d} }

func (e *Exporter) Kind() string  { return KindExport }
func (e *Exporter) Title() string { return texts.ExportKind }

func (e *Exporter) Run(ctx context.Context, job jobsdomain.Job, p jobsservice.Progress) error {
	path, err := e.run(ctx, job, p)
	if err != nil {
		return exportStop(ctx, err)
	}
	return p.Save(ctx, "{}", fmt.Sprintf(texts.ExportDone, path))
}

func (e *Exporter) run(ctx context.Context, job jobsdomain.Job, p jobsservice.Progress) (string, error) {
	cfg, err := e.d.Config.Get(ctx)
	if err != nil {
		return "", err
	}
	src, err := e.d.Sources.Get(ctx, job.SourceID)
	if err != nil {
		return "", err
	}
	settings, err := e.d.Settings.Get(ctx)
	if err != nil {
		return "", err
	}
	people, err := e.d.Store.People(ctx, job.SourceID, cfg.IncludeQueries)
	if err != nil {
		return "", err
	}
	tpl := Template(src.Title, cfg.Columns)
	model := ""
	rows := make([]tables.Row, 0, len(people))
	for i, person := range people {
		row, err := e.row(ctx, tpl, person, &model)
		if err != nil {
			return "", err
		}
		rows = append(rows, row)
		if err := p.Save(ctx, "{}", fmt.Sprintf(texts.ExportPeople, i+1, len(people))); err != nil {
			return "", err
		}
	}
	out := tpl
	out.Columns = append(append([]tables.Column{}, tpl.Columns...), tables.Column{Name: texts.TableComments, Type: tables.TypeText})
	return e.d.Write(settings.ExportsDir, out, rows)
}

// row — строка человека: из кэша, если колонки и тексты не менялись, иначе — моделью.
func (e *Exporter) row(ctx context.Context, tpl tables.Template, person domain.Person, model *string) (tables.Row, error) {
	views, err := e.d.Mentions.PersonMentions(ctx, person.ID)
	if err != nil {
		return tables.Row{}, err
	}
	docs, err := e.d.Store.DocTexts(ctx, person.ID)
	if err != nil {
		return tables.Row{}, err
	}
	pages := e.pages(person, docs, views)
	hash := inputHash(tpl, pages)
	row, ok, err := e.d.Store.Row(ctx, person.ID, hash)
	if err != nil {
		return tables.Row{}, err
	}
	if !ok {
		if row, err = e.extract(ctx, tpl, pages, model); err != nil {
			return tables.Row{}, err
		}
		if err := e.d.Store.SaveRow(ctx, person.ID, hash, row); err != nil {
			return tables.Row{}, err
		}
	}
	row = validate.Rows(tpl, []tables.Row{row})[0]
	comments := e.comments(views)
	row.Cells[texts.TableComments] = tables.Cell{Value: comments, Typed: comments, Confidence: "high"}
	row.File = firstLink(views)
	return row, nil
}

func (e *Exporter) extract(ctx context.Context, tpl tables.Template, pages []recognize.PageText, model *string) (tables.Row, error) {
	if *model == "" {
		m, err := e.d.Models.Pick(ctx)
		if err != nil {
			return tables.Row{}, err
		}
		*model = m
	}
	ex := &recognize.Extractor{LLM: e.d.LLM, Model: *model, TextOnly: true}
	rows, err := ex.Extract(ctx, tpl, "", pages, false)
	if err != nil {
		return tables.Row{}, err
	}
	if len(rows) == 0 {
		return tables.Row{Cells: map[string]tables.Cell{}}, nil
	}
	return rows[0], nil
}

// pages — вход модели: известные поля человека, тексты его документов, сообщения о нём.
func (e *Exporter) pages(person domain.Person, docs []string, views []pdomain.MentionView) []recognize.PageText {
	known := fmt.Sprintf("Известно о человеке из разбора группы:\nФИО: %s\nИИН: %s\nНомер документа: %s\nДата рождения: %s",
		person.FullName, person.IIN, person.DocNumber, person.BirthDate)
	pages := []recognize.PageText{{Page: normalize.Page{Index: 1}, Text: known}}
	for _, d := range docs {
		pages = append(pages, recognize.PageText{Page: normalize.Page{Index: len(pages) + 1}, Text: "Документ:\n" + d})
	}
	var chat strings.Builder
	for _, v := range views {
		fmt.Fprintf(&chat, "%s: %s\n", v.Author, v.Text)
	}
	return append(pages, recognize.PageText{Page: normalize.Page{Index: len(pages) + 1}, Text: "Сообщения о человеке:\n" + chat.String()})
}

// comments — все сообщения о человеке: «дата · группа · автор: текст», по строке на сообщение.
func (e *Exporter) comments(views []pdomain.MentionView) string {
	lines := make([]string, 0, len(views))
	for _, v := range views {
		date := time.Unix(v.Date, 0).In(e.d.Loc).Format("02.01.2006 15:04")
		lines = append(lines, fmt.Sprintf("%s · %s · %s: %s", date, v.SourceTitle, v.Author, strings.TrimSpace(v.Text)))
	}
	return strings.Join(lines, "\n")
}

// firstLink — ссылка на первое сообщение о человеке в супергруппе ("" — ссылок нет).
func firstLink(views []pdomain.MentionView) string {
	for _, v := range views {
		var peer tgdomain.Peer
		if json.Unmarshal([]byte(v.SourceCfg), &peer) == nil {
			if link := peer.MessageLink(v.MsgID); link != "" {
				return link
			}
		}
	}
	return ""
}

func inputHash(tpl tables.Template, pages []recognize.PageText) string {
	h := sha256.New()
	h.Write([]byte(tpl.Hash() + recognize.PromptsVersion()))
	for _, p := range pages {
		h.Write([]byte{0})
		h.Write([]byte(p.Text))
	}
	return hex.EncodeToString(h.Sum(nil)[:12])
}

// exportStop: нет модели или Ollama — «ждёт вас»; прочее — русская причина.
func exportStop(ctx context.Context, err error) error {
	switch {
	case ctx.Err() != nil:
		return err
	case errors.Is(err, analyze.ErrNoModel), errors.Is(err, ollama.ErrNotRunning):
		return &jobsservice.WaitUserError{Reason: texts.AnalyzeNeedModel}
	default:
		return fmt.Errorf("%s: %w", texts.ExportFailed, err)
	}
}
