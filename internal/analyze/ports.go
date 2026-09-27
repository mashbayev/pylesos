// Файл: internal/analyze/ports.go
package analyze

import (
	"context"
	"errors"
	"fmt"

	docdomain "pylesos/internal/docforms/domain"
	"pylesos/internal/normalize"
	"pylesos/internal/ollama"
	sourcesdomain "pylesos/internal/sources/domain"
	tgdomain "pylesos/internal/telegram/domain"
	tgrepo "pylesos/internal/telegram/repository"
)

// Gateway — скачивание вложений аккаунтом Telegram (gotdgw).
type Gateway interface {
	Media(ctx context.Context, p tgdomain.Peer, msgID int) ([]tgdomain.File, error)
}

// Messages — сообщения и вложения группы (telegram/repository).
type Messages interface {
	PendingAnalysis(ctx context.Context, sourceID int64, limit int) ([]tgdomain.Message, error)
	Message(ctx context.Context, sourceID int64, msgID int) (tgdomain.Message, bool, error)
	MarkAnalyzed(ctx context.Context, sourceID int64, msgID int, errText string) error
	PendingMedia(ctx context.Context, sourceID int64, limit int) ([]tgrepo.MediaRef, error)
	SaveRecognition(ctx context.Context, sourceID int64, msgID, idx int, text, fields, model string) error
	MediaFields(ctx context.Context, sourceID int64, msgID int) ([]map[string]string, error)
}

type Sources interface {
	Get(ctx context.Context, id int64) (sourcesdomain.Source, error)
}

type Pages interface {
	Bytes(name string, data []byte) ([]normalize.Page, error)
}

type Forms interface {
	List(ctx context.Context) ([]docdomain.Form, error)
}

// ModelPicker — какой моделью распознавать.
type ModelPicker interface {
	Pick(ctx context.Context) (string, error)
}

var ErrNoModel = errors.New("нет модели распознавания")

// VisionModels — установленные модели Ollama, понимающие изображения.
type VisionModels interface {
	VisionModels(ctx context.Context) ([]string, error)
}

// OllamaPicker выбирает модель Ollama: gemma4:e4b, иначе первую подходящую.
type OllamaPicker struct{ Ollama VisionModels }

func (p OllamaPicker) Pick(ctx context.Context) (string, error) {
	models, err := p.Ollama.VisionModels(ctx)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrNoModel, err)
	}
	if m := ollama.Preferred(models); m != "" {
		return m, nil
	}
	return "", ErrNoModel
}
