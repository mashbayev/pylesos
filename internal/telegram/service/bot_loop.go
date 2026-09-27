// Файл: internal/telegram/service/bot_loop.go
package service

import (
	"context"
	"errors"
	"strconv"
	"time"

	"pylesos/internal/events"
	"pylesos/internal/telegram/botapi"
	"pylesos/internal/telegram/repository"
)

const (
	botPollTimeout   = 30 // секунд long polling
	botConflictPause = time.Minute
	botRetryFirst    = 5 * time.Second
	botRetryMax      = 5 * time.Minute
)

// Run — цикл бота до выхода из программы: сеанс с текущим токеном; новый токен — новый сеанс.
func (b *Bot) Run(ctx context.Context) {
	defer close(b.done)
	for ctx.Err() == nil {
		wait, restarted := b.runSession(ctx)
		if wait && !restarted && ctx.Err() == nil {
			b.waitRestart(ctx) // нет токена или он неверный — ждём нового
		}
	}
}

// runSession — сеанс, который прерывается новым токеном; restarted — прерван именно им
// (сигнал уже получен, ждать его снова нельзя — ревью M2.1, №1).
func (b *Bot) runSession(ctx context.Context) (wait, restarted bool) {
	sessCtx, cancel := context.WithCancel(ctx)
	watched := make(chan bool, 1)
	go func() {
		select {
		case <-b.restart:
			cancel()
			watched <- true
		case <-sessCtx.Done():
			watched <- false
		}
	}()
	wait = b.session(sessCtx)
	cancel()
	return wait, <-watched
}

func (b *Bot) waitRestart(ctx context.Context) {
	select {
	case <-b.restart:
	case <-ctx.Done():
	}
}

// session — один токен; true — продолжать нечего, пока не введут новый токен.
func (b *Bot) session(ctx context.Context) bool {
	token, ok, err := b.d.Secrets.Get(ctx, secretBotToken)
	if err != nil || !ok {
		if err != nil {
			b.d.Log.Error("токен бота", "err", err)
		}
		b.setStatus(BotOff, "")
		return true
	}
	client := b.d.Factory(string(token))
	b.setStatus(BotStarting, "")
	me, err := b.hello(ctx, client)
	if errors.Is(err, botapi.ErrUnauthorized) {
		b.setStatus(BotBadToken, "")
		return true
	}
	if err != nil {
		return true // сеанс отменён: выход из программы или новый токен
	}
	b.setStatus(BotRunning, me.Username)
	return b.poll(ctx, client, me.Username)
}

// hello — getMe с растущей паузой без связи (ревью M2.1, №6); 401 и отмена — сразу.
func (b *Bot) hello(ctx context.Context, client BotClient) (botapi.User, error) {
	backoff := botRetryFirst
	for {
		me, err := client.GetMe(ctx)
		if err == nil || errors.Is(err, botapi.ErrUnauthorized) || ctx.Err() != nil {
			return me, err
		}
		if err := b.retry(ctx, err, backoff); err != nil {
			return botapi.User{}, err
		}
		backoff = min(backoff*2, botRetryMax)
	}
}

// poll — long polling до отмены; ошибки связи, 409 и сбои записи — пауза и повтор.
func (b *Bot) poll(ctx context.Context, client BotClient, username string) bool {
	backoff := botRetryFirst
	for ctx.Err() == nil {
		ups, err := client.GetUpdates(ctx, b.offset(ctx), botPollTimeout)
		switch {
		case ctx.Err() != nil:
			return false
		case errors.Is(err, botapi.ErrUnauthorized):
			b.setStatus(BotBadToken, "")
			return true
		case errors.Is(err, botapi.ErrConflict):
			b.setStatus(BotConflict, username)
			if b.d.Sleep(ctx, botConflictPause) != nil {
				return false
			}
			continue
		case err != nil:
			if b.retry(ctx, err, backoff) != nil {
				return false
			}
			backoff = min(backoff*2, botRetryMax)
			b.probe(ctx, client, username)
			continue
		}
		b.setStatus(BotRunning, username)
		if err := b.handle(ctx, ups); err != nil {
			b.d.Log.Error("сохранить сообщения бота", "err", err)
			if b.d.Sleep(ctx, backoff) != nil { // порция повторится, без долбёжки (ревью M2.1, №2)
				return false
			}
			backoff = min(backoff*2, botRetryMax)
			continue
		}
		backoff = botRetryFirst
	}
	return false
}

// probe — быстрая проверка связи после обрыва: иначе «нет связи» висело бы до конца long polling (до 30 с).
func (b *Bot) probe(ctx context.Context, client BotClient, username string) {
	if _, err := client.GetMe(ctx); err == nil {
		b.setStatus(BotRunning, username)
	}
}

func (b *Bot) retry(ctx context.Context, err error, wait time.Duration) error {
	b.setStatus(BotOffline, "")
	b.d.Log.Warn("бот: нет связи с Telegram", "err", err)
	return b.d.Sleep(ctx, wait)
}

// handle сохраняет сообщения порции и сдвигает offset (после записи — чтобы не потерять при сбое).
func (b *Bot) handle(ctx context.Context, ups []botapi.Update) error {
	if len(ups) == 0 {
		return nil
	}
	bySource, err := b.mapUpdates(ctx, ups)
	if err != nil {
		return err
	}
	for sourceID, msgs := range bySource {
		if err := b.d.Messages.Save(ctx, sourceID, msgs, repository.ViaBot, b.d.Now().Unix()); err != nil {
			return err
		}
	}
	next := ups[len(ups)-1].UpdateID + 1
	if err := b.d.KV.Put(ctx, map[string]string{settingBotOffset: strconv.FormatInt(next, 10)}); err != nil {
		return err
	}
	if len(bySource) > 0 {
		b.d.Events.Publish(events.Jobs())
	}
	return nil
}

// offset — с какого обновления читать; не прочитался или испорчен — с начала (ревью M2.1, №8).
func (b *Bot) offset(ctx context.Context) int64 {
	all, err := b.d.KV.All(ctx)
	if err != nil {
		b.d.Log.Error("offset бота", "err", err)
		return 0
	}
	raw := all[settingBotOffset]
	if raw == "" {
		return 0
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		b.d.Log.Warn("испорченный offset бота — читаю с начала", "value", raw)
		return 0
	}
	return n
}
