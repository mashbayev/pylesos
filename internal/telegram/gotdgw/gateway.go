// Файл: internal/telegram/gotdgw/gateway.go
package gotdgw

import (
	"context"
	"log/slog"
	"time"

	"github.com/gotd/td/telegram"

	"pylesos/internal/telegram/domain"
)

// connectTimeout — сколько вызов ждёт подключения; дальше — «нет связи с Telegram» (подключение продолжается в фоне).
const connectTimeout = 15 * time.Second

// KeyLoader — api_id и api_hash (вводит пользователь, П19); ok=false — ещё не введены.
type KeyLoader func(ctx context.Context) (appID int, appHash string, ok bool, err error)

// Gateway — одно соединение с Telegram на программу; поднимается при первом обращении.
type Gateway struct {
	keys    KeyLoader
	storage *SessionStorage
	log     *slog.Logger
	conns   *connector[*telegram.Client]
}

func New(base context.Context, keys KeyLoader, storage *SessionStorage, log *slog.Logger) *Gateway {
	g := &Gateway{keys: keys, storage: storage, log: log}
	g.conns = newConnector[*telegram.Client](base, g.dial, connectTimeout)
	return g
}

// Reset разрывает соединение: следующий вызов подключится с новыми ключами.
func (g *Gateway) Reset() { g.conns.reset() }

func (g *Gateway) conn(ctx context.Context) (*telegram.Client, error) {
	c, err := g.conns.get(ctx)
	if err != nil {
		return nil, mapErr(err)
	}
	return c, nil
}

// dial создаёт клиента по сохранённым ключам; соединение держит run до отмены контекста попытки.
func (g *Gateway) dial(ctx context.Context) (*telegram.Client, runFunc, error) {
	appID, appHash, ok, err := g.keys(ctx)
	if err != nil {
		return nil, nil, err
	}
	if !ok {
		return nil, nil, domain.ErrNotConfigured
	}
	client := telegram.NewClient(appID, appHash, telegram.Options{SessionStorage: g.storage, NoUpdates: true})
	run := func(ctx context.Context, onReady func()) error {
		err := client.Run(ctx, func(ctx context.Context) error {
			onReady()
			<-ctx.Done()
			return ctx.Err()
		})
		if err != nil && ctx.Err() == nil {
			g.log.Warn("соединение с Telegram закрыто", "err", err)
		}
		return err
	}
	return client, run, nil
}
