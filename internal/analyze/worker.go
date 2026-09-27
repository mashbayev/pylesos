// Файл: internal/analyze/worker.go
package analyze

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	jobsdomain "pylesos/internal/jobs/domain"
	jobsservice "pylesos/internal/jobs/service"
	"pylesos/internal/llm"
	"pylesos/internal/ollama"
	tgdomain "pylesos/internal/telegram/domain"
	"pylesos/internal/texts"
)

// KindAnalyze — вид задания «разбор Telegram».
const KindAnalyze = "tg_analyze"

type Deps struct {
	Gateway  Gateway
	Messages Messages
	People   People
	Sources  Sources
	Pages    Pages
	Forms    Forms
	LLM      llm.Responder
	Models   ModelPicker
	Now      func() time.Time
}

// Worker — задание разбора группы: сначала документы, затем сообщения (повтор берёт только новое).
type Worker struct {
	d      Deps
	docs   *DocReader
	finder *Finder
	linker *Linker
}

func NewWorker(d Deps) *Worker {
	return &Worker{d: d, docs: &DocReader{LLM: d.LLM, Pages: d.Pages, Forms: d.Forms},
		finder: &Finder{LLM: d.LLM}, linker: &Linker{People: d.People, Now: d.Now}}
}

func (w *Worker) Kind() string  { return KindAnalyze }
func (w *Worker) Title() string { return texts.AnalyzeKind }

const (
	batchDocs = 5
	batchMsgs = 20
)

type run struct {
	w      *Worker
	source int64
	peer   tgdomain.Peer
	model  string
	p      jobsservice.Progress
	docs   int
	msgs   int
}

func (w *Worker) Run(ctx context.Context, job jobsdomain.Job, p jobsservice.Progress) error {
	r, err := w.begin(ctx, job, p)
	if err != nil {
		return stopReason(ctx, err)
	}
	for more := true; more; {
		if more, err = r.docsBatch(ctx); err != nil {
			return stopReason(ctx, err)
		}
	}
	for more := true; more; {
		if more, err = r.msgsBatch(ctx); err != nil {
			return stopReason(ctx, err)
		}
	}
	return p.Save(ctx, "{}", texts.AnalyzeDone)
}

func (w *Worker) begin(ctx context.Context, job jobsdomain.Job, p jobsservice.Progress) (*run, error) {
	src, err := w.d.Sources.Get(ctx, job.SourceID)
	if err != nil {
		return nil, err
	}
	var peer tgdomain.Peer
	if err := json.Unmarshal([]byte(src.Config), &peer); err != nil {
		return nil, fmt.Errorf("группа источника %d: %w", job.SourceID, err)
	}
	model, err := w.d.Models.Pick(ctx)
	if err != nil {
		return nil, err
	}
	return &run{w: w, source: job.SourceID, peer: peer, model: model, p: p}, nil
}

// stopReason: нет модели или Ollama — «ждёт вас»; нет входа или доступа к группе — тоже; прочее — русская причина.
func stopReason(ctx context.Context, err error) error {
	switch {
	case ctx.Err() != nil:
		return err
	case errors.Is(err, ErrNoModel), errors.Is(err, ollama.ErrNotRunning):
		return &jobsservice.WaitUserError{Reason: texts.AnalyzeNeedModel}
	case errors.Is(err, tgdomain.ErrNotAuthorized), errors.Is(err, tgdomain.ErrNotConfigured):
		return &jobsservice.WaitUserError{Reason: texts.TgNeedLogin}
	case errors.Is(err, tgdomain.ErrGroupUnavailable):
		return &jobsservice.WaitUserError{Reason: texts.TgGroupUnavailable}
	default:
		return fmt.Errorf("%s: %w", texts.AnalyzeFailed, err)
	}
}
