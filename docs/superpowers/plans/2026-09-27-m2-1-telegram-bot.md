# M2.1 — Бот для новых сообщений. План реализации

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Пользователь вставляет на вкладке «Telegram» токен бота от @BotFather — и пока программа запущена, новые и исправленные сообщения добавленных супергрупп сразу попадают в базу (без дублей с аккаунтом), на экране виден статус бота.

**Architecture:** `internal/telegram/botapi` — минимальный клиент Bot API на `net/http` (`getMe`, `getUpdates` long polling), без сторонних библиотек. `internal/telegram/service/bot*.go` — служба `Bot`: цикл long polling, перевод сообщений Bot API в `domain.Message`, запись через тот же репозиторий с `via='bot'`, смещение `offset` в таблице `settings`, токен — в зашифрованных секретах. Бот не задание: он работает, пока открыта программа (П6, П10), и не мешает заданию `tg_sync` на том же источнике. Отметку сверки аккаунтом (`tg_sync_state`) бот не трогает — пропуски за время простоя догружает аккаунт (03-telegram, грабли).

**Tech Stack:** Go 1.26, `net/http`, `encoding/json`, SQLite; React.

**Spec:** `docs/specs/2026-09-26-pylesos-design.md` — раздел 3 «Telegram — бот (новые сообщения)», раздел 6 (экран «Telegram»/«Настройки»: бот — токен, статус), раздел 7 («Неверный токен бота — статус бота в настройках»). Правила — `CLAUDE.md`, `.claude/rules/03-telegram.md`, `10-security.md`.

## Global Constraints

- Всё из M1/M2 в силе (коммиты запрещены, лимиты 400/50, `// Файл:`, без глобальных переменных кроме ошибок-меток и `//go:embed`, ошибки не игнорируются, тексты — `internal/texts`).
- Токен бота вводит пользователь (П19), хранится только в `secrets` (ключ `telegram_bot_token`), в журнал и JSON не попадает; адрес запроса содержит токен — его не логировать.
- Бот принимает сообщения **только супергрупп**, уже добавленных как источники: `chat_id` бота = `-100` + id канала (03-telegram). Обычные группы и чужие чаты — игнорировать.
- Служебные сообщения (вступил/вышел, закреп и т.п.) не сохранять: сохраняются сообщения с текстом, подписью, фото или документом.
- Смена токена сбрасывает `offset` (другой бот — другие номера обновлений).
- Сетевых запросов к настоящему Telegram в тестах нет: клиент Bot API тестируется на `httptest`.

## Review Focus

1. **Одно и то же сообщение пришло и от бота, и от аккаунта** — одна строка, ничего не затёрто. Тест: Task 2, `TestBotSavesOnlyKnownSupergroups` (повторная запись через репозиторий M2 с COALESCE).
2. **Неверный токен** — статус «Telegram не принял токен бота», цикл не долбит сервер; после нового токена бот запускается сам. Тест: Task 2, `TestBadTokenThenNewToken`.
3. **Нет связи / 409 (вебхук или вторая копия)** — понятный статус, повтор с паузой, программа не падает. Тест: Task 2, `TestConflictAndNetworkRetry`.
4. **Выход из программы во время long polling** — цикл бота завершается сразу, база не закрывается под ним. Тест: Task 2, `TestRunStopsOnCancel`; порядок остановки — Task 4.
5. **Исправленное сообщение** (`edited_message`) — текст обновляется. Тест: Task 2, `TestEditedMessageUpdatesText`.

---

### Task 1: Клиент Bot API

**Files:**
- Create: `internal/telegram/botapi/types.go`, `internal/telegram/botapi/client.go`
- Test: `internal/telegram/botapi/client_test.go`

**Interfaces:**
- Produces: `botapi.New(base, token string, hc *http.Client) *Client` (`base` — `https://api.telegram.org` или адрес тестового сервера); `(*Client).GetMe(ctx) (User, error)`; `(*Client).GetUpdates(ctx, offset int64, timeoutSec int) ([]Update, error)`; `botapi.ErrUnauthorized` (401), `botapi.ErrConflict` (409); типы `User{ID int64; FirstName, LastName, Username string}`, `Chat{ID int64; Type, Title string}`, `Update{UpdateID int64; Message, EditedMessage *Message}`, `Message{MessageID int; Date, EditDate int64; Chat Chat; From *User; SenderChat *Chat; Text, Caption string; ReplyTo *MessageRef; MessageThreadID int; IsTopic bool; ForwardOrigin *ForwardOrigin; Photo []PhotoSize; Document *Document; Raw json.RawMessage}`, `MessageRef{MessageID int}`, `ForwardOrigin{Type string; SenderUser *User; SenderUserName string; Chat, SenderChat *Chat}`, `PhotoSize{FileID string; FileSize int64}`, `Document{FileID, FileName, MimeType string; FileSize int64}`

- [ ] **Step 1: Написать падающие тесты**

`internal/telegram/botapi/client_test.go`:
```go
// Файл: internal/telegram/botapi/client_test.go
package botapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const token = "123456:ТЕСТОВЫЙ-токен"

func server(t *testing.T, handler func(w http.ResponseWriter, r *http.Request)) *Client {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(handler))
	t.Cleanup(ts.Close)
	return New(ts.URL, token, ts.Client())
}

func write(t *testing.T, w http.ResponseWriter, code int, body string) {
	t.Helper()
	w.WriteHeader(code)
	if _, err := w.Write([]byte(body)); err != nil {
		t.Errorf("ответ: %v", err)
	}
}

func TestGetMe(t *testing.T) {
	c := server(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/bot"+token+"/getMe" {
			write(t, w, 404, `{"ok":false,"error_code":404,"description":"Not Found"}`)
			return
		}
		write(t, w, 200, `{"ok":true,"result":{"id":42,"first_name":"Пылесос","username":"pylesos_test_bot"}}`)
	})
	me, err := c.GetMe(context.Background())
	if err != nil || me.ID != 42 || me.Username != "pylesos_test_bot" {
		t.Fatalf("%+v %v", me, err)
	}
}

func TestGetUpdatesParsesMessages(t *testing.T) {
	body := `{"ok":true,"result":[
	 {"update_id":10,"message":{"message_id":5,"date":1700000000,"chat":{"id":-1001234567890,"type":"supergroup","title":"Группа"},
	  "from":{"id":7,"first_name":"Иван","last_name":"Тестов"},"caption":"Подпись к фото",
	  "reply_to_message":{"message_id":3,"date":1,"chat":{"id":-1001234567890,"type":"supergroup"}},
	  "forward_origin":{"type":"hidden_user","sender_user_name":"Другой автопарк"},
	  "photo":[{"file_id":"small","file_size":100},{"file_id":"big","file_size":900}]}},
	 {"update_id":11,"edited_message":{"message_id":6,"date":1700000001,"edit_date":1700000100,
	  "chat":{"id":-1001234567890,"type":"supergroup"},"text":"Исправлено",
	  "document":{"file_id":"doc","file_name":"удостоверение.pdf","mime_type":"application/pdf","file_size":2048}}}]}`
	c := server(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("offset") != "10" || r.URL.Query().Get("timeout") != "30" ||
			!strings.Contains(r.URL.Query().Get("allowed_updates"), "edited_message") {
			write(t, w, 400, `{"ok":false,"error_code":400,"description":"bad query"}`)
			return
		}
		write(t, w, 200, body)
	})
	ups, err := c.GetUpdates(context.Background(), 10, 30)
	if err != nil || len(ups) != 2 {
		t.Fatalf("%+v %v", ups, err)
	}
	m := ups[0].Message
	if m.MessageID != 5 || m.Chat.ID != -1001234567890 || m.From.FirstName != "Иван" || m.Caption != "Подпись к фото" ||
		m.ReplyTo.MessageID != 3 || m.ForwardOrigin.SenderUserName != "Другой автопарк" || len(m.Photo) != 2 || len(m.Raw) == 0 {
		t.Fatalf("сообщение: %+v", m)
	}
	e := ups[1].EditedMessage
	if e.EditDate != 1700000100 || e.Text != "Исправлено" || e.Document.FileName != "удостоверение.pdf" {
		t.Fatalf("исправленное: %+v", e)
	}
}

func TestErrors(t *testing.T) {
	cases := map[int]error{401: ErrUnauthorized, 409: ErrConflict}
	for code, want := range cases {
		c := server(t, func(w http.ResponseWriter, _ *http.Request) {
			write(t, w, code, `{"ok":false,"error_code":`+http.StatusText(0)+`0,"description":"x"}`)
		})
		if _, err := c.GetUpdates(context.Background(), 0, 0); !errors.Is(err, want) {
			t.Fatalf("%d: %v", code, err)
		}
	}
	c := server(t, func(w http.ResponseWriter, _ *http.Request) {
		write(t, w, 500, `не JSON`)
	})
	_, err := c.GetMe(context.Background())
	if err == nil || strings.Contains(err.Error(), token) {
		t.Fatalf("ошибка без токена в тексте: %v", err)
	}
}
```

- [ ] **Step 2: Запустить — должен упасть**

Run: `go test ./internal/telegram/botapi/`
Expected: FAIL — `undefined: New`.

- [ ] **Step 3: Реализовать**

`internal/telegram/botapi/types.go`:
```go
// Файл: internal/telegram/botapi/types.go
// Package botapi — минимальный клиент Telegram Bot API: getMe и getUpdates (long polling).
package botapi

import "encoding/json"

type User struct {
	ID        int64  `json:"id"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Username  string `json:"username"`
}

type Chat struct {
	ID    int64  `json:"id"`
	Type  string `json:"type"` // private, group, supergroup, channel
	Title string `json:"title"`
}

type MessageRef struct {
	MessageID int `json:"message_id"`
}

type ForwardOrigin struct {
	Type           string `json:"type"`
	SenderUser     *User  `json:"sender_user"`
	SenderUserName string `json:"sender_user_name"`
	Chat           *Chat  `json:"chat"`
	SenderChat     *Chat  `json:"sender_chat"`
}

type PhotoSize struct {
	FileID   string `json:"file_id"`
	FileSize int64  `json:"file_size"`
}

type Document struct {
	FileID   string `json:"file_id"`
	FileName string `json:"file_name"`
	MimeType string `json:"mime_type"`
	FileSize int64  `json:"file_size"`
}

type Message struct {
	MessageID       int             `json:"message_id"`
	Date            int64           `json:"date"`
	EditDate        int64           `json:"edit_date"`
	Chat            Chat            `json:"chat"`
	From            *User           `json:"from"`
	SenderChat      *Chat           `json:"sender_chat"`
	Text            string          `json:"text"`
	Caption         string          `json:"caption"`
	ReplyTo         *MessageRef     `json:"reply_to_message"`
	MessageThreadID int             `json:"message_thread_id"`
	IsTopic         bool            `json:"is_topic_message"`
	ForwardOrigin   *ForwardOrigin  `json:"forward_origin"`
	Photo           []PhotoSize     `json:"photo"`
	Document        *Document       `json:"document"`
	Raw             json.RawMessage `json:"-"` // сообщение как пришло
}

// UnmarshalJSON сохраняет исходный JSON сообщения в Raw.
func (m *Message) UnmarshalJSON(data []byte) error {
	type plain Message
	var p plain
	if err := json.Unmarshal(data, &p); err != nil {
		return err
	}
	*m = Message(p)
	m.Raw = append(json.RawMessage(nil), data...)
	return nil
}

type Update struct {
	UpdateID      int64    `json:"update_id"`
	Message       *Message `json:"message"`
	EditedMessage *Message `json:"edited_message"`
}
```

`internal/telegram/botapi/client.go`:
```go
// Файл: internal/telegram/botapi/client.go
package botapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

var (
	ErrUnauthorized = errors.New("Bot API: токен не принят (401)")
	ErrConflict     = errors.New("Bot API: обновления уже получает кто-то другой (409)")
)

// Client — запросы к Bot API. Токен есть в адресе запроса: адрес не логируется и не попадает в ошибки.
type Client struct {
	base  string
	token string
	http  *http.Client
}

func New(base, token string, hc *http.Client) *Client { return &Client{base: base, token: token, http: hc} }

type envelope struct {
	OK          bool            `json:"ok"`
	Result      json.RawMessage `json:"result"`
	ErrorCode   int             `json:"error_code"`
	Description string          `json:"description"`
}

func (c *Client) GetMe(ctx context.Context) (User, error) {
	var me User
	err := c.call(ctx, "getMe", url.Values{}, &me)
	return me, err
}

// GetUpdates — long polling: ждёт до timeoutSec секунд новые сообщения и правки.
func (c *Client) GetUpdates(ctx context.Context, offset int64, timeoutSec int) ([]Update, error) {
	q := url.Values{}
	q.Set("offset", strconv.FormatInt(offset, 10))
	q.Set("timeout", strconv.Itoa(timeoutSec))
	q.Set("allowed_updates", `["message","edited_message"]`)
	ups := []Update{}
	err := c.call(ctx, "getUpdates", q, &ups)
	return ups, err
}

func (c *Client) call(ctx context.Context, method string, q url.Values, out any) error {
	endpoint := c.base + "/bot" + c.token + "/" + method + "?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("Bot API %s: запрос", method)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("Bot API %s: %w", method, unwrapURL(err))
	}
	var env envelope
	decodeErr := json.NewDecoder(resp.Body).Decode(&env)
	if err := resp.Body.Close(); err != nil {
		return fmt.Errorf("Bot API %s: закрыть ответ: %w", method, err)
	}
	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return ErrUnauthorized
	case resp.StatusCode == http.StatusConflict:
		return ErrConflict
	case decodeErr != nil:
		return fmt.Errorf("Bot API %s: ответ %d не разобран: %w", method, resp.StatusCode, decodeErr)
	case !env.OK:
		return fmt.Errorf("Bot API %s: %d %s", method, env.ErrorCode, env.Description)
	}
	if err := json.Unmarshal(env.Result, out); err != nil {
		return fmt.Errorf("Bot API %s: результат: %w", method, err)
	}
	return nil
}

// unwrapURL убирает из ошибки *url.Error адрес (в нём токен), оставляя причину.
func unwrapURL(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		return ue.Err
	}
	return err
}
```

- [ ] **Step 4: Запустить — должен пройти**

Run: `go test -race ./internal/telegram/botapi/`
Expected: PASS.

- [ ] **Step 5: Проверка**

Run: `go vet ./... && go test ./...`
Expected: без ошибок.

---

### Task 2: Служба бота

**Files:**
- Modify: `internal/texts/ru.go` (тексты бота)
- Create: `internal/telegram/service/bot.go`, `internal/telegram/service/bot_loop.go`, `internal/telegram/service/bot_map.go`
- Test: `internal/telegram/service/bot_test.go`

**Interfaces:**
- Consumes: `botapi.*` (Task 1), `SecretStore` (M2), `repository.Save/ViaBot` (M2), `sourcesdomain` (M2), `events.Event` (M1)
- Produces:
  - `service.BotClient` — `GetMe(ctx) (botapi.User, error)`, `GetUpdates(ctx, offset int64, timeoutSec int) ([]botapi.Update, error)`; `service.BotFactory func(token string) BotClient`
  - `service.KV` — `All(ctx) (map[string]string, error)`, `Put(ctx, map[string]string) error` (таблица `settings`)
  - `service.MessageSaver` — `Save(ctx, sourceID int64, msgs []domain.Message, via string, at int64) error`
  - `service.BotSources` — `ListByKind(ctx, sourcesdomain.Kind) ([]sourcesdomain.Source, error)`
  - `service.BotDeps{Secrets SecretStore; Factory BotFactory; KV KV; Sources BotSources; Messages MessageSaver; Events Publisher; Now func() time.Time; Sleep func(context.Context, time.Duration) error; Log *slog.Logger}`, где `Publisher{Publish(events.Event)}`
  - `service.NewBot(BotDeps) *Bot`; `(*Bot).Run(ctx)` (блокирует до отмены ctx); `(*Bot).Done() <-chan struct{}`; `(*Bot).SetToken(ctx, token string) (problem string, err error)`; `(*Bot).Status() BotStatus`
  - `service.BotState` (`BotOff "off"`, `BotStarting "starting"`, `BotRunning "running"`, `BotBadToken "bad_token"`, `BotConflict "conflict"`, `BotOffline "offline"`), `service.BotStatus{State BotState; Username string}`
  - `service.FieldBotToken = "bot_token"`, `service.botChatID(peerID int64) int64` (`-100…`)

- [ ] **Step 1: Тексты**

Дописать в `internal/texts/ru.go`:
```go
// Telegram-бот (M2.1).
const (
	TgBotTitle         = "Бот для новых сообщений"
	TgBotHint          = "Создайте бота у @BotFather, вставьте сюда его токен и добавьте бота в каждую группу администратором — тогда новые сообщения будут приходить сразу, пока программа открыта."
	TgBotToken         = "Токен бота"
	TgBotSave          = "Сохранить токен"
	TgBotOff           = "Бот не подключён"
	TgBotStarting      = "Бот подключается…"
	TgBotRunning       = "Бот @%s принимает новые сообщения"
	TgBotBadToken      = "Telegram не принял токен бота — проверьте его и сохраните заново"
	TgBotConflict      = "Этого бота уже слушает другая программа (вебхук или вторая копия) — повторю через минуту"
	TgBotOffline       = "Нет связи с Telegram — бот повторит попытку сам"
	TgBotSaved         = "Токен сохранён, бот подключается"
	ErrTgBotTokenFormat = "Токен выглядит так: 123456789:AAE… — скопируйте его из @BotFather целиком"
)
```

- [ ] **Step 2: Написать падающие тесты**

`internal/telegram/service/bot_test.go`:
```go
// Файл: internal/telegram/service/bot_test.go
package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"pylesos/internal/events"
	sourcesdomain "pylesos/internal/sources/domain"
	sourcesrepo "pylesos/internal/sources/repository"
	settingsrepo "pylesos/internal/settings/repository"
	"pylesos/internal/store/storetest"
	"pylesos/internal/telegram/botapi"
	"pylesos/internal/telegram/domain"
	tgrepo "pylesos/internal/telegram/repository"
	"pylesos/internal/texts"
)

const goodToken = "123456789:AAEabcdefghijklmnopqrstuvwxyz012345"

// fakeBotClient — Bot API в памяти: очередь ответов getUpdates.
type fakeBotClient struct {
	mu      sync.Mutex
	meErr   error
	batches [][]botapi.Update
	errs    []error
	offsets []int64
}

func (f *fakeBotClient) GetMe(context.Context) (botapi.User, error) {
	return botapi.User{ID: 1, Username: "pylesos_test_bot"}, f.meErr
}

func (f *fakeBotClient) GetUpdates(ctx context.Context, offset int64, _ int) ([]botapi.Update, error) {
	f.mu.Lock()
	f.offsets = append(f.offsets, offset)
	if len(f.errs) > 0 {
		err := f.errs[0]
		f.errs = f.errs[1:]
		f.mu.Unlock()
		return nil, err
	}
	if len(f.batches) > 0 {
		b := f.batches[0]
		f.batches = f.batches[1:]
		f.mu.Unlock()
		return b, nil
	}
	f.mu.Unlock()
	<-ctx.Done() // long polling без новых сообщений
	return nil, ctx.Err()
}

type botEnv struct {
	bot    *Bot
	client *fakeBotClient
	msgs   *tgrepo.SQLite
	kv     *settingsrepo.SQLite
	source int64
	slept  chan time.Duration
	tokens chan string
}

func newBotEnv(t *testing.T, withToken bool) *botEnv {
	t.Helper()
	db := storetest.Open(t)
	sources := sourcesrepo.NewSQLite(db)
	cfg, err := json.Marshal(domain.Peer{Type: domain.PeerChannel, ID: 1234567890, AccessHash: 5})
	if err != nil {
		t.Fatal(err)
	}
	id, _, err := sources.Ensure(context.Background(), sourcesdomain.Source{Kind: sourcesdomain.KindTelegramGroup,
		Title: "Супергруппа", Config: string(cfg)})
	if err != nil {
		t.Fatal(err)
	}
	e := &botEnv{client: &fakeBotClient{}, msgs: tgrepo.NewSQLite(db), kv: settingsrepo.NewSQLite(db), source: id,
		slept: make(chan time.Duration, 10), tokens: make(chan string, 10)}
	secrets := fakeSecrets{}
	if withToken {
		secrets[secretBotToken] = []byte(goodToken)
	}
	e.bot = NewBot(BotDeps{Secrets: secrets, Factory: func(token string) BotClient { e.tokens <- token; return e.client },
		KV: e.kv, Sources: sources, Messages: e.msgs, Events: events.NewBus(), Now: func() time.Time { return time.Unix(9000, 0) },
		Sleep: func(ctx context.Context, d time.Duration) error { e.slept <- d; return ctx.Err() },
		Log: slog.New(slog.NewJSONHandler(io.Discard, nil))})
	return e
}

func (e *botEnv) start(t *testing.T) context.CancelFunc {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	go e.bot.Run(ctx)
	t.Cleanup(func() {
		cancel()
		<-e.bot.Done()
	})
	return cancel
}

func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("не дождались: %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func (e *botEnv) count(t *testing.T) int {
	t.Helper()
	counts, err := e.msgs.Counts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return counts[e.source]
}

func upd(id int64, chat int64, msgID int, text string) botapi.Update {
	return botapi.Update{UpdateID: id, Message: &botapi.Message{MessageID: msgID, Date: 100, Text: text,
		Chat: botapi.Chat{ID: chat, Type: "supergroup"}, From: &botapi.User{ID: 7, FirstName: "Тестовый"}}}
}

func TestBotSavesOnlyKnownSupergroups(t *testing.T) {
	e := newBotEnv(t, true)
	known := botChatID(1234567890)
	service := upd(4, known, 12, "")
	e.client.batches = [][]botapi.Update{{
		upd(1, known, 10, "Новое сообщение о водителе"),
		upd(2, -1009999999999, 11, "чужая группа"),
		upd(3, -555, 13, "обычная группа"),
		service, // вступил в группу — без текста и вложений
	}}
	e.start(t)
	waitFor(t, "сообщение сохранено", func() bool { return e.count(t) == 1 })
	waitFor(t, "offset сохранён", func() bool {
		all, err := e.kv.All(context.Background())
		return err == nil && all[settingBotOffset] == "5"
	})
	if st := e.bot.Status(); st.State != BotRunning || st.Username != "pylesos_test_bot" {
		t.Fatalf("статус %+v", st)
	}
	if err := e.msgs.Save(context.Background(), e.source, []domain.Message{{ID: 10, Date: 100}}, tgrepo.ViaAccount, 1); err != nil {
		t.Fatalf("то же сообщение от аккаунта: %v", err)
	}
	if e.count(t) != 1 {
		t.Fatal("дубль после записи аккаунтом")
	}
}

func TestEditedMessageUpdatesText(t *testing.T) {
	e := newBotEnv(t, true)
	known := botChatID(1234567890)
	edited := upd(2, known, 10, "Исправленный текст")
	edited.EditedMessage, edited.Message = edited.Message, nil
	edited.EditedMessage.EditDate = 200
	e.client.batches = [][]botapi.Update{{upd(1, known, 10, "Первый текст")}, {edited}}
	e.start(t)
	waitFor(t, "правка применена", func() bool {
		ids, err := e.msgs.LiveIDsAfter(context.Background(), e.source, 0, 10)
		if err != nil || len(ids) != 1 {
			return false
		}
		return e.textOf(t, 10) == "Исправленный текст"
	})
}

func (e *botEnv) textOf(t *testing.T, id int) string {
	t.Helper()
	var s string
	if err := storetestDB(e).Get(&s, `SELECT text FROM tg_messages WHERE source_id = ? AND msg_id = ?`, e.source, id); err != nil {
		return ""
	}
	return s
}

func TestBadTokenThenNewToken(t *testing.T) {
	e := newBotEnv(t, true)
	e.client.meErr = botapi.ErrUnauthorized
	e.start(t)
	waitFor(t, "статус «неверный токен»", func() bool { return e.bot.Status().State == BotBadToken })
	if problem, err := e.bot.SetToken(context.Background(), "не токен"); err != nil || problem != texts.ErrTgBotTokenFormat {
		t.Fatalf("формат токена: %q %v", problem, err)
	}
	e.client.meErr = nil
	if problem, err := e.bot.SetToken(context.Background(), " "+goodToken+" "); err != nil || problem != "" {
		t.Fatalf("новый токен: %q %v", problem, err)
	}
	waitFor(t, "бот запущен с новым токеном", func() bool { return e.bot.Status().State == BotRunning })
}

func TestConflictAndNetworkRetry(t *testing.T) {
	e := newBotEnv(t, true)
	e.client.errs = []error{botapi.ErrConflict, errors.New("dial tcp: no route to host")}
	e.start(t)
	if d := <-e.slept; d != botConflictPause {
		t.Fatalf("пауза после 409: %v", d)
	}
	if d := <-e.slept; d != botRetryFirst {
		t.Fatalf("пауза без связи: %v", d)
	}
	waitFor(t, "снова работает", func() bool { return e.bot.Status().State == BotRunning })
}

func TestRunStopsOnCancelAndWithoutToken(t *testing.T) {
	e := newBotEnv(t, false)
	cancel := e.start(t)
	waitFor(t, "статус «не подключён»", func() bool { return e.bot.Status().State == BotOff })
	cancel()
	select {
	case <-e.bot.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("цикл бота не остановился после выхода из программы")
	}
}

func TestBotChatID(t *testing.T) {
	if botChatID(1234567890) != -1001234567890 {
		t.Fatalf("%d", botChatID(1234567890))
	}
}
```

И в конец `bot_test.go` — доступ к базе для `textOf`:
```go
// storetestDB — база окружения (для проверки текста напрямую).
func storetestDB(e *botEnv) *sqlx.DB { return e.db }
```
а в `botEnv` добавить поле `db *sqlx.DB` и в `newBotEnv` заполнить `db: db` (импорт `"github.com/jmoiron/sqlx"`).

- [ ] **Step 3: Запустить — должен упасть**

Run: `go test ./internal/telegram/service/ -run Bot`
Expected: FAIL — `undefined: NewBot` и др.

- [ ] **Step 4: Реализовать**

`internal/telegram/service/bot.go`:
```go
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

func (b *Bot) setStatus(s BotState, username string) {
	b.mu.Lock()
	b.status = BotStatus{State: s, Username: username}
	b.mu.Unlock()
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
```

`internal/telegram/service/bot_loop.go`:
```go
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
		sessCtx, cancel := context.WithCancel(ctx)
		go func() {
			select {
			case <-b.restart:
				cancel()
			case <-sessCtx.Done():
			}
		}()
		wait := b.session(sessCtx)
		cancel()
		if wait && ctx.Err() == nil {
			b.waitRestart(ctx) // нет токена или он неверный — ждём нового
		}
	}
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
	me, err := client.GetMe(ctx)
	if errors.Is(err, botapi.ErrUnauthorized) {
		b.setStatus(BotBadToken, "")
		return true
	}
	if err != nil {
		return b.retry(ctx, err, botRetryFirst) != nil
	}
	b.setStatus(BotRunning, me.Username)
	return b.poll(ctx, client, me.Username)
}

// poll — long polling до отмены; ошибки связи и 409 — пауза и повтор.
func (b *Bot) poll(ctx context.Context, client BotClient, username string) bool {
	backoff := botRetryFirst
	for ctx.Err() == nil {
		offset, err := b.offset(ctx)
		if err != nil {
			b.d.Log.Error("offset бота", "err", err)
			return false
		}
		ups, err := client.GetUpdates(ctx, offset, botPollTimeout)
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
			continue
		}
		backoff = botRetryFirst
		b.setStatus(BotRunning, username)
		if err := b.handle(ctx, ups); err != nil {
			b.d.Log.Error("сохранить сообщения бота", "err", err)
		}
	}
	return false
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
		b.d.Events.Publish(events.Event{Type: events.JobsChanged})
	}
	return nil
}

func (b *Bot) offset(ctx context.Context) (int64, error) {
	all, err := b.d.KV.All(ctx)
	if err != nil {
		return 0, err
	}
	raw := all[settingBotOffset]
	if raw == "" {
		return 0, nil
	}
	return strconv.ParseInt(raw, 10, 64)
}
```

`internal/telegram/service/bot_map.go`:
```go
// Файл: internal/telegram/service/bot_map.go
package service

import (
	"context"
	"encoding/json"
	"strings"

	sourcesdomain "pylesos/internal/sources/domain"
	"pylesos/internal/telegram/botapi"
	"pylesos/internal/telegram/domain"
)

// botChatID — chat_id супергруппы в Bot API: «-100» и id канала (03-telegram, грабли).
func botChatID(channelID int64) int64 { return -1_000_000_000_000 - channelID }

// mapUpdates раскладывает сообщения по источникам; чужие чаты, обычные группы и служебные сообщения — пропуск.
func (b *Bot) mapUpdates(ctx context.Context, ups []botapi.Update) (map[int64][]domain.Message, error) {
	chats, err := b.knownChats(ctx)
	if err != nil {
		return nil, err
	}
	out := map[int64][]domain.Message{}
	for _, u := range ups {
		m := u.Message
		if m == nil {
			m = u.EditedMessage
		}
		sourceID, ok := chats[chatIDOf(m)]
		if !ok || !meaningful(m) {
			continue
		}
		out[sourceID] = append(out[sourceID], fromBot(m))
	}
	return out, nil
}

func chatIDOf(m *botapi.Message) int64 {
	if m == nil {
		return 0
	}
	return m.Chat.ID
}

// knownChats — chat_id бота → источник, только для супергрупп.
func (b *Bot) knownChats(ctx context.Context) (map[int64]int64, error) {
	list, err := b.d.Sources.ListByKind(ctx, sourcesdomain.KindTelegramGroup)
	if err != nil {
		return nil, err
	}
	chats := map[int64]int64{}
	for _, s := range list {
		var p domain.Peer
		if json.Unmarshal([]byte(s.Config), &p) == nil && p.Type == domain.PeerChannel {
			chats[botChatID(p.ID)] = s.ID
		}
	}
	return chats, nil
}

// meaningful — есть текст, подпись, фото или документ (служебные «вступил в группу» — нет).
func meaningful(m *botapi.Message) bool {
	return m.Text != "" || m.Caption != "" || len(m.Photo) > 0 || m.Document != nil
}

func fromBot(m *botapi.Message) domain.Message {
	out := domain.Message{ID: m.MessageID, Date: m.Date, EditDate: m.EditDate, Text: m.Text, Raw: string(m.Raw)}
	if out.Text == "" {
		out.Text = m.Caption
	}
	out.AuthorID, out.AuthorName = botAuthor(m)
	if m.ReplyTo != nil {
		out.ReplyTo = m.ReplyTo.MessageID
	}
	if m.IsTopic {
		out.ThreadTop = m.MessageThreadID
	}
	out.FwdFrom = botForward(m.ForwardOrigin)
	out.Media = botMedia(m)
	return out
}

func botAuthor(m *botapi.Message) (int64, string) {
	if m.SenderChat != nil {
		return m.SenderChat.ID, m.SenderChat.Title
	}
	if m.From != nil {
		return m.From.ID, botUserName(m.From)
	}
	return 0, ""
}

func botUserName(u *botapi.User) string {
	if name := strings.TrimSpace(u.FirstName + " " + u.LastName); name != "" {
		return name
	}
	if u.Username != "" {
		return "@" + u.Username
	}
	return ""
}

func botForward(o *botapi.ForwardOrigin) string {
	switch {
	case o == nil:
		return ""
	case o.SenderUserName != "":
		return o.SenderUserName
	case o.SenderUser != nil:
		return botUserName(o.SenderUser)
	case o.Chat != nil:
		return o.Chat.Title
	case o.SenderChat != nil:
		return o.SenderChat.Title
	}
	return ""
}

// botMedia — сведения о вложении; файл по ним не скачивается (П12) — на разборе его возьмёт аккаунт по номеру сообщения.
func botMedia(m *botapi.Message) []domain.Media {
	if len(m.Photo) > 0 {
		largest := m.Photo[len(m.Photo)-1]
		return []domain.Media{{Kind: domain.MediaPhoto, MimeType: "image/jpeg", Size: largest.FileSize}}
	}
	if m.Document != nil {
		return []domain.Media{{Kind: domain.MediaDocument, MimeType: m.Document.MimeType,
			FileName: m.Document.FileName, Size: m.Document.FileSize}}
	}
	return nil
}
```

- [ ] **Step 5: Запустить — должен пройти**

Run: `go test -race ./internal/telegram/service/`
Expected: PASS (включая тесты M2).

- [ ] **Step 6: Проверка**

Run: `go vet ./... && go test ./...`
Expected: без ошибок.

---

### Task 3: Экран и HTTP — токен и статус бота

**Files:**
- Modify: `internal/screens/types.go`, `internal/screens/builder.go` (порт `TelegramBot`, поле `Deps.Bot`), `internal/screens/telegram.go`
- Test: `internal/screens/telegram_test.go`
- Modify: `internal/api/server.go`, `internal/api/handlers_telegram.go`, Test: `internal/api/telegram_test.go`, `internal/api/api_test.go`
- Modify: `web/src/types.ts`, `web/src/pages/TelegramPage.tsx`

**Interfaces:**
- Consumes: `tgservice.BotStatus/BotState*/FieldBotToken` (Task 2)
- Produces:
  - `screens.TelegramBot{Status() tgservice.BotStatus; SetToken(ctx, token string) (string, error)}`; `screens.Deps.Bot`
  - `TelegramScreen` + `BotTitle, BotHint, BotStatus, BotSaveLabel string; BotField Field` (JSON `bot_title, bot_hint, bot_status, bot_save_label, bot_field`); всегда заполнены (бот независим от входа)
  - `(*Builder).TelegramBot(ctx, token string) (TelegramScreen, error)`; `api.Screens.TelegramBot`; маршрут `POST /api/telegram/bot` (`{"token":"..."}`)

- [ ] **Step 1: Написать падающие тесты**

В `internal/screens/telegram_test.go` добавить подставку и тест, а в `tgBuilder` и `NewBuilder(Deps{...})` из `TestHomeTelegramRows` и `builder()` в `screens_test.go` — поле `Bot: &fakeBot{}`:
```go
type fakeBot struct {
	status  tgservice.BotStatus
	problem string
	token   string
}

func (f *fakeBot) Status() tgservice.BotStatus { return f.status }
func (f *fakeBot) SetToken(_ context.Context, token string) (string, error) {
	f.token = token
	return f.problem, nil
}

func TestTelegramBotSection(t *testing.T) {
	bot := &fakeBot{status: tgservice.BotStatus{State: tgservice.BotRunning, Username: "pylesos_bot"}}
	b := NewBuilder(Deps{Jobs: fakeJobs{}, Kinds: fakeKinds{}, Settings: &fakeSettings{}, Sources: fakeSources{},
		Counts: fakeCounts{}, Stats: fakeStats{}, Login: &fakeLogin{state: tgservice.LoginState{Step: tgservice.StepKeys}},
		Groups: &fakeGroups{}, Bot: bot, Loc: time.UTC})
	s, err := b.Telegram(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if s.BotTitle != texts.TgBotTitle || s.BotStatus != fmt.Sprintf(texts.TgBotRunning, "pylesos_bot") ||
		s.BotField.Kind != "password" || s.BotField.Value != "" {
		t.Fatalf("раздел бота при шаге ключей: %+v", s)
	}
	bot.problem = texts.ErrTgBotTokenFormat
	s, err = b.TelegramBot(context.Background(), "мусор")
	if err != nil || bot.token != "мусор" || s.BotField.Error != texts.ErrTgBotTokenFormat {
		t.Fatalf("ошибка токена: %+v %v", s.BotField, err)
	}
	bot.problem, bot.status = "", tgservice.BotStatus{State: tgservice.BotBadToken}
	s, err = b.TelegramBot(context.Background(), "123:abc")
	if err != nil || s.Message != texts.TgBotSaved || s.BotStatus != texts.TgBotBadToken {
		t.Fatalf("после сохранения: %+v %v", s, err)
	}
}
```

В `internal/api/api_test.go` в `fakeScreens` добавить:
```go
func (f *fakeScreens) TelegramBot(_ context.Context, token string) (screens.TelegramScreen, error) {
	f.saved = map[string]string{"token": token}
	return screens.TelegramScreen{Message: texts.TgBotSaved}, nil
}
```
В конец `TestTelegramRoutes` (`internal/api/telegram_test.go`):
```go
	do(t, http.MethodPost, ts.URL+"/api/telegram/bot", "секрет", `{"token":"123:abc"}`)
	if fs.saved["token"] != "123:abc" {
		t.Fatalf("токен бота не передан: %v", fs.saved)
	}
```

Run: `go test ./internal/screens/ ./internal/api/` → Expected: FAIL (`unknown field Bot`, нет `TelegramBot`).

- [ ] **Step 2: Реализовать**

`internal/screens/types.go` — в `TelegramScreen` добавить:
```go
	BotTitle     string `json:"bot_title"`
	BotHint      string `json:"bot_hint"`
	BotStatus    string `json:"bot_status"`
	BotSaveLabel string `json:"bot_save_label"`
	BotField     Field  `json:"bot_field"`
```

`internal/screens/builder.go` — порт и поле:
```go
type TelegramBot interface {
	Status() tgservice.BotStatus
	SetToken(ctx context.Context, token string) (string, error)
}
```
и в `Deps` — `Bot TelegramBot`.

`internal/screens/telegram.go` — добавить:
```go
// TelegramBot — сохранить токен бота; ошибка формата — у поля.
func (b *Builder) TelegramBot(ctx context.Context, token string) (TelegramScreen, error) {
	problem, err := b.d.Bot.SetToken(ctx, token)
	if err != nil {
		return TelegramScreen{}, fmt.Errorf("токен бота: %w", err)
	}
	if problem != "" {
		s, err := b.telegram(ctx, nil, nil, "", false)
		s.BotField.Error = problem
		return s, err
	}
	return b.telegram(ctx, nil, nil, texts.TgBotSaved, false)
}

func (b *Builder) botSection(s *TelegramScreen) {
	s.BotTitle, s.BotHint, s.BotSaveLabel = texts.TgBotTitle, texts.TgBotHint, texts.TgBotSave
	s.BotField = Field{Key: tgservice.FieldBotToken, Label: texts.TgBotToken, Kind: "password"}
	st := b.d.Bot.Status()
	switch st.State {
	case tgservice.BotRunning:
		s.BotStatus = fmt.Sprintf(texts.TgBotRunning, st.Username)
	case tgservice.BotStarting:
		s.BotStatus = texts.TgBotStarting
	case tgservice.BotBadToken:
		s.BotStatus = texts.TgBotBadToken
	case tgservice.BotConflict:
		s.BotStatus = texts.TgBotConflict
	case tgservice.BotOffline:
		s.BotStatus = texts.TgBotOffline
	default:
		s.BotStatus = texts.TgBotOff
	}
}
```
и в `telegram(...)` сразу после создания `s` — вызов `b.botSection(&s)`.

`internal/api/server.go` — в `Screens` добавить `TelegramBot(ctx context.Context, token string) (screens.TelegramScreen, error)`; маршрут `mux.HandleFunc("POST /api/telegram/bot", s.handleTelegramBot)`.

`internal/api/handlers_telegram.go` — добавить:
```go
func (s *Server) handleTelegramBot(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, texts.ErrBadRequest)
		return
	}
	screen, err := s.deps.Screens.TelegramBot(r.Context(), req.Token)
	s.writeScreen(w, "токен бота", screen, err)
}
```

`web/src/types.ts` — в `TelegramScreen` добавить `bot_title: string; bot_hint: string; bot_status: string; bot_save_label: string; bot_field: Field`.

`web/src/pages/TelegramPage.tsx` — добавить компонент и вывести его в конце страницы (`<BotSection screen={s} />`):
```tsx
function BotSection({ screen }: { screen: TelegramScreen }) {
  const [token, setToken] = useState('')
  const save = useTelegramAction(() => '/api/telegram/bot', (t: string) => ({ token: t }))
  const f = screen.bot_field
  return (
    <form className="max-w-xl space-y-2 border-t border-neutral-200 pt-4 dark:border-neutral-800"
      onSubmit={(e) => { e.preventDefault(); save.mutate(token) }}>
      <h2 className="font-semibold">{screen.bot_title}</h2>
      <p className="text-sm text-neutral-500">{screen.bot_hint}</p>
      <p>{screen.bot_status}</p>
      <label className="block space-y-1">
        <span className="text-sm">{f.label}</span>
        <input type={f.kind} value={token} onChange={(e) => setToken(e.target.value)}
          className="w-full rounded-md border border-neutral-300 bg-transparent px-2 py-1 dark:border-neutral-700" />
        {f.error && <span className="text-sm text-red-600">{f.error}</span>}
      </label>
      {save.error && <p className="text-red-600">{save.error.message}</p>}
      <Button type="submit" disabled={save.isPending}>{save.isPending ? screen.saving_label : screen.bot_save_label}</Button>
    </form>
  )
}
```

- [ ] **Step 3: Запустить — должен пройти**

Run: `go test -race ./internal/screens/ ./internal/api/ && cd web && npm run typecheck && npm run build && touch dist/.keep`
Expected: PASS; типы и сборка без ошибок.

- [ ] **Step 4: Проверка**

Run: `go vet ./internal/... && go test ./internal/...`
Expected: без ошибок (`cmd/pylesos` — в Task 4).

---

### Task 4: Запуск бота вместе с программой

**Files:**
- Modify: `cmd/pylesos/telegram.go`, `cmd/pylesos/wire.go`, `cmd/pylesos/serve.go`

**Interfaces:**
- Consumes: `tgservice.NewBot`, `botapi.New`, `settingsrepo.NewSQLite` (M1)
- Produces: бот работает, пока открыта программа; при выходе цикл бота останавливается до закрытия базы

- [ ] **Step 1: Сборка зависимостей**

В `cmd/pylesos/telegram.go`: в `telegramParts` добавить поле `store *secrets.Store` и заполнить его (`store: store`).

В `cmd/pylesos/wire.go`: в `app` добавить поле `bot *tgservice.Bot`; после создания `groups`/`login`:
```go
	bot := tgservice.NewBot(tgservice.BotDeps{Secrets: tg.store,
		Factory: func(token string) tgservice.BotClient {
			return botapi.New("https://api.telegram.org", token, &http.Client{Timeout: 60 * time.Second})
		},
		KV: settingsrepo.NewSQLite(db), Sources: tg.sources, Messages: tg.msgs, Events: bus,
		Now: time.Now, Sleep: tgservice.SleepCtx, Log: log})
	go bot.Run(ctx)
```
в `screens.Deps` — `Bot: bot`; в возврат — `app{server: server, runner: runner, bot: bot}` (импорты `net/http`, `pylesos/internal/telegram/botapi`). Если `wire` становится длиннее 50 строк — вынести создание бота в функцию `newBot(ctx, db, tg, bus, log) *tgservice.Bot` в `cmd/pylesos/telegram.go`.

В `cmd/pylesos/serve.go` после `a.runner.Wait()` добавить `<-a.bot.Done()` (цикл бота завершается по отмене `appCtx`; база закрывается после).

- [ ] **Step 2: Полная сборка и проверки**

Run: `scripts/build.sh && go test -race ./... && GOOS=windows go vet ./... && GOOS=darwin go vet ./...`
Expected: `Готово: bin/pylesos`; всё PASS. Настоящий бот — только по просьбе пользователя.

- [ ] **Step 3: База знаний**

Обновить (продолжение П24, пользователь просил доделывать до готовности): `road_map.md` (M2.1 ✅), `CURRENT_STATE.md`, `08-development-stages.md`, грабли `03-telegram.md` (бот — только супергруппы; offset после записи; смена токена сбрасывает offset).
