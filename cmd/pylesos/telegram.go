// Файл: cmd/pylesos/telegram.go
package main

import (
	"context"
	"log/slog"
	"net/http"
	"path/filepath"
	"time"

	"github.com/jmoiron/sqlx"

	"pylesos/internal/events"
	"pylesos/internal/paths"
	"pylesos/internal/secrets"
	settingsrepo "pylesos/internal/settings/repository"
	sourcesrepo "pylesos/internal/sources/repository"
	"pylesos/internal/telegram/botapi"
	"pylesos/internal/telegram/gotdgw"
	tgrepo "pylesos/internal/telegram/repository"
	tgservice "pylesos/internal/telegram/service"
)

// telegramParts — модуль Telegram: шлюз, хранилища, вход, сбор.
type telegramParts struct {
	gateway *gotdgw.Gateway
	sources *sourcesrepo.SQLite
	msgs    *tgrepo.SQLite
	keys    *tgservice.KeyStore
	worker  *tgservice.SyncWorker
	store   *secrets.Store
}

func newTelegram(ctx context.Context, db *sqlx.DB, p paths.Paths, log *slog.Logger) (telegramParts, error) {
	key, err := secrets.LoadOrCreateKey(filepath.Join(p.DataDir, "secret.key"))
	if err != nil {
		return telegramParts{}, err
	}
	cipher, err := secrets.NewCipher(key)
	if err != nil {
		return telegramParts{}, err
	}
	store := secrets.NewStore(db, cipher)
	keys := tgservice.NewKeyStore(store)
	loader := func(ctx context.Context) (int, string, bool, error) {
		k, ok, err := keys.Load(ctx)
		return k.AppID, k.AppHash, ok, err
	}
	gw := gotdgw.New(ctx, loader, gotdgw.NewSessionStorage(store), log)
	sources := sourcesrepo.NewSQLite(db)
	msgs := tgrepo.NewSQLite(db)
	worker := tgservice.NewSyncWorker(gw, msgs, sources, time.Now, tgservice.SleepCtx, 100, 1000)
	return telegramParts{gateway: gw, sources: sources, msgs: msgs, keys: keys, worker: worker, store: store}, nil
}

// newBot — бот для новых сообщений (M2.1): работает, пока программа запущена.
func newBot(ctx context.Context, db *sqlx.DB, tg telegramParts, bus *events.Bus, log *slog.Logger) *tgservice.Bot {
	bot := tgservice.NewBot(tgservice.BotDeps{Secrets: tg.store,
		Factory: func(token string) tgservice.BotClient {
			return botapi.New("https://api.telegram.org", token, &http.Client{Timeout: 60 * time.Second})
		},
		KV: settingsrepo.NewSQLite(db), Sources: tg.sources, Messages: tg.msgs, Events: bus,
		Now: time.Now, Sleep: tgservice.SleepCtx, Log: log})
	go bot.Run(ctx)
	return bot
}
