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

	"github.com/jmoiron/sqlx"

	"pylesos/internal/events"
	settingsrepo "pylesos/internal/settings/repository"
	sourcesdomain "pylesos/internal/sources/domain"
	sourcesrepo "pylesos/internal/sources/repository"
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
	db     *sqlx.DB
	bot    *Bot
	client *fakeBotClient
	msgs   *tgrepo.SQLite
	kv     *settingsrepo.SQLite
	source int64
	slept  chan time.Duration
	tokens chan string
	block  bool // Sleep ждёт отмены (долгая пауза), а не возвращается сразу
	pub    *recBus
}

// recBus — какие экраны просили перечитать.
type recBus struct {
	mu      sync.Mutex
	screens []string
}

func (r *recBus) Publish(e events.Event) {
	r.mu.Lock()
	r.screens = append(r.screens, e.Screens...)
	r.mu.Unlock()
}

func (r *recBus) has(screen string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, s := range r.screens {
		if s == screen {
			return true
		}
	}
	return false
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
	e := &botEnv{db: db, client: &fakeBotClient{}, msgs: tgrepo.NewSQLite(db), kv: settingsrepo.NewSQLite(db), source: id,
		slept: make(chan time.Duration, 100), tokens: make(chan string, 10), pub: &recBus{}}
	secrets := fakeSecrets{}
	if withToken {
		secrets[secretBotToken] = []byte(goodToken)
	}
	e.bot = NewBot(BotDeps{Secrets: secrets, Factory: func(token string) BotClient { e.tokens <- token; return e.client },
		KV: e.kv, Sources: sources, Messages: e.msgs, Events: e.pub, Now: func() time.Time { return time.Unix(9000, 0) },
		Sleep: func(ctx context.Context, d time.Duration) error {
			select {
			case e.slept <- d:
			default:
			}
			if e.block {
				<-ctx.Done()
			}
			return ctx.Err()
		},
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

func waitUntil(t *testing.T, what string, ok func() bool) {
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
	waitUntil(t, "сообщение сохранено", func() bool { return e.count(t) == 1 })
	waitUntil(t, "offset сохранён", func() bool {
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
	var row struct {
		Text   string `db:"text"`
		Author string `db:"author_name"`
		Via    string `db:"via"`
	}
	if err := e.db.Get(&row, `SELECT text, author_name, via FROM tg_messages WHERE source_id = ? AND msg_id = 10`, e.source); err != nil {
		t.Fatal(err)
	}
	if row.Text != "Новое сообщение о водителе" || row.Author != "Тестовый" || row.Via != "bot" {
		t.Fatalf("запись аккаунтом затёрла данные бота: %+v", row)
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
	waitUntil(t, "правка применена", func() bool {
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
	waitUntil(t, "статус «неверный токен»", func() bool { return e.bot.Status().State == BotBadToken })
	if problem, err := e.bot.SetToken(context.Background(), "не токен"); err != nil || problem != texts.ErrTgBotTokenFormat {
		t.Fatalf("формат токена: %q %v", problem, err)
	}
	e.client.meErr = nil
	if problem, err := e.bot.SetToken(context.Background(), " "+goodToken+" "); err != nil || problem != "" {
		t.Fatalf("новый токен: %q %v", problem, err)
	}
	waitUntil(t, "бот запущен с новым токеном", func() bool { return e.bot.Status().State == BotRunning })
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
	waitUntil(t, "снова работает", func() bool { return e.bot.Status().State == BotRunning })
}

func TestRunStopsOnCancelAndWithoutToken(t *testing.T) {
	e := newBotEnv(t, false)
	cancel := e.start(t)
	waitUntil(t, "статус «не подключён»", func() bool { return e.bot.Status().State == BotOff })
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

// storetestDB — база окружения (для проверки текста напрямую).
func storetestDB(e *botEnv) *sqlx.DB { return e.db }

// Ревью M2.1, №1: новый токен во время паузы «нет связи» запускает бота, а не теряется.
func TestNewTokenDuringOfflinePause(t *testing.T) {
	e := newBotEnv(t, true)
	e.block = true
	e.client.meErr = errors.New("dial tcp: no route to host")
	e.start(t)
	<-e.slept // бот в паузе после неудачного getMe
	e.client.meErr = nil
	if problem, err := e.bot.SetToken(context.Background(), goodToken); err != nil || problem != "" {
		t.Fatalf("%q %v", problem, err)
	}
	waitUntil(t, "бот запущен после нового токена", func() bool { return e.bot.Status().State == BotRunning })
}

// Ревью M2.1, №6: без связи пауза перед getMe растёт.
func TestGetMeBackoffGrows(t *testing.T) {
	e := newBotEnv(t, true)
	e.client.meErr = errors.New("dial tcp: no route to host")
	e.start(t)
	first, second := <-e.slept, <-e.slept
	if first != botRetryFirst || second != 2*botRetryFirst {
		t.Fatalf("паузы %v, %v", first, second)
	}
}

// Ревью M2.1, №2: ошибка записи — пауза, а не долбёжка getUpdates.
func TestSaveErrorPauses(t *testing.T) {
	e := newBotEnv(t, true)
	e.bot.d.Messages = failingSaver{}
	known := botChatID(1234567890)
	e.client.batches = [][]botapi.Update{{upd(1, known, 10, "текст")}, {upd(1, known, 10, "текст")}}
	e.start(t)
	if d := <-e.slept; d != botRetryFirst {
		t.Fatalf("пауза после ошибки записи: %v", d)
	}
}

type failingSaver struct{}

func (failingSaver) Save(context.Context, int64, []domain.Message, string, int64) error {
	return errors.New("база занята")
}

// Ревью M2.1, №3: смена статуса бота просит перечитать экран «Telegram».
func TestStatusChangeRefreshesTelegramScreen(t *testing.T) {
	e := newBotEnv(t, true)
	e.start(t)
	waitUntil(t, "событие для экрана Telegram", func() bool { return e.pub.has("telegram") })
}

// Ревью M2.1, №4: автор-канал записывается так же, как у аккаунта (id канала без «-100»).
func TestSenderChatIDLikeAccount(t *testing.T) {
	m := &botapi.Message{MessageID: 1, Text: "x", SenderChat: &botapi.Chat{ID: -1001234567890, Title: "Канал"}}
	if got := fromBot(m); got.AuthorID != 1234567890 || got.AuthorName != "Канал" {
		t.Fatalf("%+v", got)
	}
}

// Ревью M2.1, №8: испорченный offset не зацикливает бота.
func TestBrokenOffsetStartsFromZero(t *testing.T) {
	e := newBotEnv(t, true)
	if err := e.kv.Put(context.Background(), map[string]string{settingBotOffset: "мусор"}); err != nil {
		t.Fatal(err)
	}
	e.start(t)
	waitUntil(t, "getUpdates с offset 0", func() bool {
		e.client.mu.Lock()
		defer e.client.mu.Unlock()
		return len(e.client.offsets) > 0 && e.client.offsets[0] == 0
	})
}
