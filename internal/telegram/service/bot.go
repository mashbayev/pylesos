// Файл: internal/telegram/service/bot.go
package service

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"pylesos/internal/events"
	sourcesdomain "pylesos/internal/sources/domain"
	"pylesos/internal/telegram/botapi"
	"pylesos/internal/telegram/domain"
	"pylesos/internal/texts"
)

const (
	secretBotToken   = "telegram_bot_token"
	settingBotOffset = "tg_bot_offset"
	FieldBotToken    = "bot_token"
)

type BotClient interface {
	GetMe(ctx context.Context) (botapi.User, error)
	GetUpdates(ctx context.Context, offset int64, timeoutSec int) ([]botapi.Update, error)
}

// BotFactory — клиент Bot API для токена.
type BotFactory func(token string) BotClient

// KV — служебные значения в таблице settings (смещение обновлений бота).
type KV interface {
	All(ctx context.Context) (map[string]string, error)
	Put(ctx context.Context, values map[string]string) error
}

type MessageSaver interface {
	Save(ctx context.Context, sourceID int64, msgs []domain.Message, via string, at int64) error
}

type BotSources interface {
	ListByKind(ctx context.Context, kind sourcesdomain.Kind) ([]sourcesdomain.Source, error)
}

type Publisher interface{ Publish(events.Event) }

type BotDeps struct {
	Secrets  SecretStore
	Factory  BotFactory
	KV       KV
	Sources  BotSources
	Messages MessageSaver
	Events   Publisher
	Now      func() time.Time
	Sleep    func(context.Context, time.Duration) error
	Log      *slog.Logger
}

type BotState string

const (
	BotOff      BotState = "off"
	BotStarting BotState = "starting"
	BotRunning  BotState = "running"
	BotBadToken BotState = "bad_token"
	BotConflict BotState = "conflict"
	BotOffline  BotState = "offline"
)

type BotStatus struct {
	State    BotState
	Username string
}

// Bot — приём новых сообщений через Bot API, пока программа запущена (П6, П10).
type Bot struct {
	d       BotDeps
	restart chan struct{}
	done    chan struct{}

	mu     sync.Mutex
	status BotStatus
}

func NewBot(d BotDeps) *Bot {
	return &Bot{d: d, restart: make(chan struct{}, 1), done: make(chan struct{}), status: BotStatus{State: BotOff}}
}

func (b *Bot) Done() <-chan struct{} { return b.done }

func (b *Bot) Status() BotStatus {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.status
}

// setStatus меняет статус; изменение — событие для экрана «Telegram» (ревью M2.1, №3).
func (b *Bot) setStatus(s BotState, username string) {
	next := BotStatus{State: s, Username: username}
	b.mu.Lock()
	changed := b.status != next
	b.status = next
	b.mu.Unlock()
	if changed {
		b.d.Events.Publish(events.Bot())
	}
}

// SetToken проверяет формат, сохраняет токен, сбрасывает offset и перезапускает бота; problem — ошибка у поля.
func (b *Bot) SetToken(ctx context.Context, token string) (string, error) {
	token = strings.TrimSpace(token)
	if !validBotToken(token) {
		return texts.ErrTgBotTokenFormat, nil
	}
	if err := b.d.Secrets.Put(ctx, secretBotToken, []byte(token)); err != nil {
		return "", err
	}
	if err := b.d.KV.Put(ctx, map[string]string{settingBotOffset: "0"}); err != nil {
		return "", err
	}
	b.setStatus(BotStarting, "")
	select {
	case b.restart <- struct{}{}:
	default:
	}
	return "", nil
}

// validBotToken — «цифры:не меньше 30 символов из букв, цифр, _ и -».
func validBotToken(t string) bool {
	id, secret, ok := strings.Cut(t, ":")
	if !ok || id == "" || len(secret) < 30 {
		return false
	}
	for _, r := range id {
		if r < '0' || r > '9' {
			return false
		}
	}
	for _, r := range secret {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r == '_' || r == '-') {
			return false
		}
	}
	return true
}
