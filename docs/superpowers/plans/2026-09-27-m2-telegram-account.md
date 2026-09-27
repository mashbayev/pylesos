# M2 — Telegram через аккаунт. План реализации

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Пользователь вводит в окне `api_id`/`api_hash`, входит в свой аккаунт Telegram (номер → код → облачный пароль), выбирает группу — и программа выгружает всю её историю в базу, догружает новое при каждом запуске и помечает удалённые сообщения; на главной видно группы, число сообщений и дату последнего сбора.

**Architecture:** `internal/telegram` — модуль: `domain` (группа, сообщение, вложение, ошибки входа), `repository` (сообщения с COALESCE-слиянием), `service` (вход — `Login`, группы — `Groups`, сбор — `SyncWorker` вида `tg_sync` поверх заданий M1), `gotdgw` (шлюз к Telegram через `gotd/td`, одно соединение на программу, сессия — в зашифрованных секретах). Новые пакеты `internal/secrets` (AES-256-GCM, ключ в файле) и `internal/sources` (источники). Экраны: новая вкладка «Telegram», группы на «Источниках».

**Tech Stack:** Go 1.26, `github.com/gotd/td v0.162.0`, SQLite (modernc) + sqlx, React/TanStack Query.

**Spec:** `docs/specs/2026-09-26-pylesos-design.md` — разделы 2 (таблицы `sources`, `tg_messages`, `tg_media`, `secrets`), 3 («Telegram — аккаунт», «Общее»), 6 (экраны «Источники», «Настройки/Telegram»), 7 (ошибки). Проверенный API — `docs/research/gotd.md`. Правила — `CLAUDE.md`, `.claude/rules/` (03-telegram, 02-database, 10-security).

## Global Constraints

- Всё из M1 в силе: коммиты запрещены (шаг «Проверка» вместо «Commit»); файл ≤ 400 строк, функция ≤ 50; `// Файл:` первой строкой; глобальных переменных нет, кроме ошибок-меток и `//go:embed`; ошибки не игнорируются; тексты — только `internal/texts` (русский).
- Учётные данные вводит пользователь в окне (П19); `api_id`/`api_hash` и сессия — только в `secrets`, зашифрованы (10-security); ключ шифрования — файл `DataDir/secret.key`, права `0600`.
- Сбор — только через задания (`Worker`), курсор после каждой порции; FLOOD_WAIT — ждать указанное время, показывать в прогрессе (03-telegram).
- Медиа на сборе не скачиваются — только сведения о вложении (П12).
- Повторная загрузка сообщения не затирает сохранённое пустым (02-database, COALESCE).
- Реальные данные людей — не в тестах (П8): имена и тексты в тестах вымышленные.
- Сетевых обращений к Telegram в тестах нет: `Gateway` за интерфейсом; `gotdgw` покрыт тестами разбора ответов и ошибок.

## Review Focus

1. **FLOOD_WAIT посреди истории** — сбор ждёт и продолжает без дублей и пропусков. Тест: Task 5, `TestFloodWaitWaitsAndContinues`.
2. **Повторная загрузка сообщения без автора/текста** (служебные поля не пришли) — сохранённое не затирается. Тест: Task 3, `TestResaveDoesNotEraseAndOlderEditDoesNotOverwrite`.
3. **Неверный или устаревший код** — понятная ошибка у поля, можно ввести заново; устаревший — возврат к номеру. Тест: Task 4, `TestExpiredCodeReturnsToPhone`, `TestPhoneCodePasswordFlow`.
4. **Группа добавлена дважды** (двойной клик) — один источник, одно задание. Тест: Task 4, `TestAddGroupCreatesSourceOnceAndStartsSync`.
5. **Сбор без входа** (сессия отозвана) — задание «ждёт вас» с подсказкой войти, а не «ошибка». Тест: Task 5, `TestNotAuthorizedWaitsForUser`.

---

### Task 1: Миграция M2 и шифрованные секреты

**Files:**
- Create: `internal/store/migrations/0002_telegram.sql`
- Modify: `internal/store/store_test.go` (число миграций)
- Create: `internal/secrets/cipher.go`, `internal/secrets/store.go`, Test: `internal/secrets/secrets_test.go`

**Interfaces:**
- Consumes: `store.Open`, `storetest.Open` (M1)
- Produces: таблицы `secrets`, `tg_messages`, `tg_media`, индекс `sources_kind_config`; `secrets.LoadOrCreateKey(path string) ([]byte, error)`, `secrets.ErrBadKey`, `secrets.ErrCorrupted`, `secrets.NewCipher(key []byte) (*Cipher, error)`, `(*Cipher).Seal/Open`, `secrets.NewStore(db *sqlx.DB, c *Cipher) *Store`, `(*Store).Get(ctx, key) ([]byte, bool, error)`, `(*Store).Put(ctx, key string, value []byte) error`

- [ ] **Step 1: Миграция и поправка теста миграций**

`internal/store/migrations/0002_telegram.sql`:
```sql
-- Файл: internal/store/migrations/0002_telegram.sql
-- M2: секреты, сообщения и вложения Telegram. Время — Unix-секунды, ставит Go.
CREATE TABLE secrets (
    key   TEXT PRIMARY KEY,
    value BLOB NOT NULL -- nonce + шифротекст AES-256-GCM
);

-- Один источник на группу: повторное «Добавить» не создаёт второй.
CREATE UNIQUE INDEX sources_kind_config ON sources (kind, config);

CREATE TABLE tg_messages (
    source_id   INTEGER NOT NULL REFERENCES sources (id),
    msg_id      INTEGER NOT NULL,
    date        INTEGER NOT NULL,
    author_id   INTEGER NOT NULL DEFAULT 0,
    author_name TEXT    NOT NULL DEFAULT '',
    text        TEXT    NOT NULL DEFAULT '',
    reply_to    INTEGER NOT NULL DEFAULT 0,
    thread_top  INTEGER NOT NULL DEFAULT 0,
    fwd_from    TEXT    NOT NULL DEFAULT '',
    edit_date   INTEGER NOT NULL DEFAULT 0,
    deleted     INTEGER NOT NULL DEFAULT 0,
    via         TEXT    NOT NULL CHECK (via IN ('account', 'bot')),
    raw         TEXT    NOT NULL DEFAULT '',
    fetched_at  INTEGER NOT NULL,
    PRIMARY KEY (source_id, msg_id)
);

CREATE TABLE tg_media (
    source_id       INTEGER NOT NULL,
    msg_id          INTEGER NOT NULL,
    idx             INTEGER NOT NULL,
    kind            TEXT    NOT NULL CHECK (kind IN ('photo', 'document')),
    file_id         INTEGER NOT NULL,
    mime_type       TEXT    NOT NULL DEFAULT '',
    file_name       TEXT    NOT NULL DEFAULT '',
    size            INTEGER NOT NULL DEFAULT 0,
    status          TEXT    NOT NULL DEFAULT 'pending'
                    CHECK (status IN ('pending', 'downloaded', 'recognized', 'deleted')),
    recognized_text TEXT    NOT NULL DEFAULT '',
    model           TEXT    NOT NULL DEFAULT '',
    PRIMARY KEY (source_id, msg_id, idx),
    FOREIGN KEY (source_id, msg_id) REFERENCES tg_messages (source_id, msg_id)
);
```

В `internal/store/store_test.go` заменить проверку `applied != 1` на сравнение с числом файлов миграций:
```go
	names, err := fs.Glob(migrationsFS, "migrations/*.sql")
	if err != nil {
		t.Fatal(err)
	}
	if applied != len(names) {
		t.Fatalf("применено миграций: %d, файлов: %d", applied, len(names))
	}
```
(добавить импорт `io/fs`).

Run: `go test ./internal/store/`
Expected: PASS (миграции 0001 и 0002 применяются один раз).

- [ ] **Step 2: Написать падающие тесты секретов**

`internal/secrets/secrets_test.go`:
```go
// Файл: internal/secrets/secrets_test.go
package secrets

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"pylesos/internal/store/storetest"
)

func TestKeyFileCreatedOnceWithOwnerOnlyAccess(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Пылесос", "secret.key")
	first, err := LoadOrCreateKey(path)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("права ключа %v", info.Mode().Perm())
	}
	second, err := LoadOrCreateKey(path)
	if err != nil || !bytes.Equal(first, second) {
		t.Fatalf("ключ пересоздан: %v", err)
	}
	if err := os.WriteFile(path, []byte("короткий"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadOrCreateKey(path); !errors.Is(err, ErrBadKey) {
		t.Fatalf("испорченный ключ: %v", err)
	}
}

func newCipher(t *testing.T, fill byte) *Cipher {
	t.Helper()
	c, err := NewCipher(bytes.Repeat([]byte{fill}, 32))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestStoreEncryptsAndRoundTrips(t *testing.T) {
	ctx := context.Background()
	db := storetest.Open(t)
	s := NewStore(db, newCipher(t, 1))
	if _, ok, err := s.Get(ctx, "telegram_session"); err != nil || ok {
		t.Fatalf("пустое хранилище: ok=%v err=%v", ok, err)
	}
	if err := s.Put(ctx, "telegram_session", []byte("первая")); err != nil {
		t.Fatal(err)
	}
	if err := s.Put(ctx, "telegram_session", []byte("обновлённая сессия")); err != nil {
		t.Fatal(err)
	}
	got, ok, err := s.Get(ctx, "telegram_session")
	if err != nil || !ok || string(got) != "обновлённая сессия" {
		t.Fatalf("got %q ok=%v err=%v", got, ok, err)
	}
	var raw []byte
	if err := db.GetContext(ctx, &raw, `SELECT value FROM secrets WHERE key = 'telegram_session'`); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte("обновлённая")) {
		t.Fatal("секрет лежит в базе открытым текстом")
	}
	other := NewStore(db, newCipher(t, 2))
	if _, _, err := other.Get(ctx, "telegram_session"); !errors.Is(err, ErrCorrupted) {
		t.Fatalf("чужой ключ: %v", err)
	}
}
```

- [ ] **Step 3: Запустить — должен упасть**

Run: `go test ./internal/secrets/`
Expected: FAIL — `undefined: LoadOrCreateKey`, `undefined: NewStore`.

- [ ] **Step 4: Реализовать**

`internal/secrets/cipher.go`:
```go
// Файл: internal/secrets/cipher.go
// Package secrets — шифрованное хранение секретов (ключи API и сессия Telegram): AES-256-GCM (10-security.md).
package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const keySize = 32

var (
	ErrBadKey    = errors.New("файл ключа шифрования повреждён")
	ErrCorrupted = errors.New("секрет повреждён или зашифрован другим ключом")
)

// LoadOrCreateKey читает ключ шифрования; файла нет — создаёт случайный с правами 0600.
func LoadOrCreateKey(path string) ([]byte, error) {
	key, err := os.ReadFile(path)
	if err == nil {
		if len(key) != keySize {
			return nil, ErrBadKey
		}
		return key, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("прочитать ключ шифрования: %w", err)
	}
	key = make([]byte, keySize)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("создать ключ шифрования: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("папка ключа шифрования: %w", err)
	}
	if err := os.WriteFile(path, key, 0o600); err != nil {
		return nil, fmt.Errorf("записать ключ шифрования: %w", err)
	}
	return key, nil
}

type Cipher struct{ aead cipher.AEAD }

func NewCipher(key []byte) (*Cipher, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("ключ шифрования: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("режим GCM: %w", err)
	}
	return &Cipher{aead: aead}, nil
}

// Seal шифрует; результат — nonce и шифротекст.
func (c *Cipher) Seal(plain []byte) ([]byte, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("nonce: %w", err)
	}
	return c.aead.Seal(nonce, nonce, plain, nil), nil
}

// Open расшифровывает; повреждённые данные или чужой ключ — ErrCorrupted.
func (c *Cipher) Open(sealed []byte) ([]byte, error) {
	n := c.aead.NonceSize()
	if len(sealed) < n {
		return nil, ErrCorrupted
	}
	plain, err := c.aead.Open(nil, sealed[:n], sealed[n:], nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCorrupted, err)
	}
	return plain, nil
}
```

`internal/secrets/store.go`:
```go
// Файл: internal/secrets/store.go
package secrets

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jmoiron/sqlx"
)

// Store — секреты в таблице secrets; в базе только шифротекст.
type Store struct {
	db     *sqlx.DB
	cipher *Cipher
}

func NewStore(db *sqlx.DB, c *Cipher) *Store { return &Store{db: db, cipher: c} }

// Get — расшифрованное значение; ok=false — секрета нет.
func (s *Store) Get(ctx context.Context, key string) ([]byte, bool, error) {
	var sealed []byte
	err := s.db.GetContext(ctx, &sealed, `SELECT value FROM secrets WHERE key = ?`, key)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("прочитать секрет %s: %w", key, err)
	}
	plain, err := s.cipher.Open(sealed)
	if err != nil {
		return nil, false, fmt.Errorf("секрет %s: %w", key, err)
	}
	return plain, true, nil
}

func (s *Store) Put(ctx context.Context, key string, value []byte) error {
	sealed, err := s.cipher.Seal(value)
	if err != nil {
		return fmt.Errorf("зашифровать секрет %s: %w", key, err)
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO secrets (key, value) VALUES (?, ?)
		ON CONFLICT (key) DO UPDATE SET value = excluded.value`, key, sealed); err != nil {
		return fmt.Errorf("записать секрет %s: %w", key, err)
	}
	return nil
}
```

- [ ] **Step 5: Запустить — должен пройти**

Run: `go test ./internal/secrets/ ./internal/store/`
Expected: PASS.

- [ ] **Step 6: Проверка**

Run: `go vet ./... && go test ./...`
Expected: без ошибок.

---

### Task 2: Источники

**Files:**
- Create: `internal/sources/domain/source.go`, `internal/sources/repository/repository.go`, `internal/sources/repository/sqlite.go`
- Test: `internal/sources/repository/sqlite_test.go`

**Interfaces:**
- Consumes: `storetest.Open`, индекс `sources_kind_config` (Task 1)
- Produces: `domain.Kind` (`KindTelegramGroup "telegram_group"`, `KindSite "site"`), `domain.Source{ID, Kind, Title, Config, CreatedAt}`, `domain.ErrNotFound`; `repository.Repository` — `Ensure(ctx, domain.Source) (int64, bool, error)` (создать или вернуть существующий по `kind+config`; bool — создан), `Get(ctx, id int64) (domain.Source, error)`, `ListByKind(ctx, domain.Kind) ([]domain.Source, error)`; `repository.NewSQLite(db) *SQLite`

- [ ] **Step 1: Написать падающие тесты**

`internal/sources/repository/sqlite_test.go`:
```go
// Файл: internal/sources/repository/sqlite_test.go
package repository

import (
	"context"
	"errors"
	"testing"

	"pylesos/internal/sources/domain"
	"pylesos/internal/store/storetest"
)

func TestEnsureIsIdempotentPerConfig(t *testing.T) {
	ctx := context.Background()
	repo := NewSQLite(storetest.Open(t))
	a := domain.Source{Kind: domain.KindTelegramGroup, Title: "Группа А", Config: `{"id":1}`, CreatedAt: 100}
	id, created, err := repo.Ensure(ctx, a)
	if err != nil || !created {
		t.Fatalf("создание: id=%d created=%v err=%v", id, created, err)
	}
	again, created, err := repo.Ensure(ctx, a)
	if err != nil || created || again != id {
		t.Fatalf("повтор: id=%d created=%v err=%v", again, created, err)
	}
	b := domain.Source{Kind: domain.KindTelegramGroup, Title: "Группа Б", Config: `{"id":2}`, CreatedAt: 100}
	other, created, err := repo.Ensure(ctx, b)
	if err != nil || !created || other == id {
		t.Fatalf("другая группа: id=%d created=%v err=%v", other, created, err)
	}
	list, err := repo.ListByKind(ctx, domain.KindTelegramGroup)
	if err != nil || len(list) != 2 || list[0].Title != "Группа А" {
		t.Fatalf("список: %+v %v", list, err)
	}
	sites, err := repo.ListByKind(ctx, domain.KindSite)
	if err != nil || sites == nil || len(sites) != 0 {
		t.Fatalf("пустой список должен быть []: %#v %v", sites, err)
	}
	got, err := repo.Get(ctx, other)
	if err != nil || got.Config != `{"id":2}` {
		t.Fatalf("Get: %+v %v", got, err)
	}
	if _, err := repo.Get(ctx, 999); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("нет источника: %v", err)
	}
}
```

- [ ] **Step 2: Запустить — должен упасть**

Run: `go test ./internal/sources/...`
Expected: FAIL — пакеты/символы не определены.

- [ ] **Step 3: Реализовать**

`internal/sources/domain/source.go`:
```go
// Файл: internal/sources/domain/source.go
// Package domain — источники данных: Telegram-группа или площадка объявлений.
package domain

import "errors"

type Kind string

const (
	KindTelegramGroup Kind = "telegram_group"
	KindSite          Kind = "site"
)

type Source struct {
	ID        int64  `db:"id"`
	Kind      Kind   `db:"kind"`
	Title     string `db:"title"`
	Config    string `db:"config"` // JSON: для Telegram — группа (peer), для площадки — её параметры
	CreatedAt int64  `db:"created_at"`
}

var ErrNotFound = errors.New("источник не найден")
```

`internal/sources/repository/repository.go`:
```go
// Файл: internal/sources/repository/repository.go
// Package repository — хранение источников.
package repository

import (
	"context"

	"pylesos/internal/sources/domain"
)

type Repository interface {
	// Ensure создаёт источник или возвращает существующий с тем же kind и config; bool — создан.
	Ensure(ctx context.Context, s domain.Source) (int64, bool, error)
	Get(ctx context.Context, id int64) (domain.Source, error)
	ListByKind(ctx context.Context, kind domain.Kind) ([]domain.Source, error)
}
```

`internal/sources/repository/sqlite.go`:
```go
// Файл: internal/sources/repository/sqlite.go
package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jmoiron/sqlx"

	"pylesos/internal/sources/domain"
)

type SQLite struct{ db *sqlx.DB }

func NewSQLite(db *sqlx.DB) *SQLite { return &SQLite{db: db} }

func (r *SQLite) Ensure(ctx context.Context, s domain.Source) (int64, bool, error) {
	res, err := r.db.NamedExecContext(ctx, `INSERT INTO sources (kind, title, config, created_at)
		VALUES (:kind, :title, :config, :created_at) ON CONFLICT (kind, config) DO NOTHING`, s)
	if err != nil {
		return 0, false, fmt.Errorf("добавить источник: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, false, fmt.Errorf("добавить источник: %w", err)
	}
	if n == 1 {
		id, err := res.LastInsertId()
		if err != nil {
			return 0, false, fmt.Errorf("id источника: %w", err)
		}
		return id, true, nil
	}
	var id int64
	if err := r.db.GetContext(ctx, &id, `SELECT id FROM sources WHERE kind = ? AND config = ?`, s.Kind, s.Config); err != nil {
		return 0, false, fmt.Errorf("найти источник: %w", err)
	}
	return id, false, nil
}

func (r *SQLite) Get(ctx context.Context, id int64) (domain.Source, error) {
	var s domain.Source
	err := r.db.GetContext(ctx, &s, `SELECT id, kind, title, config, created_at FROM sources WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return s, domain.ErrNotFound
	}
	if err != nil {
		return s, fmt.Errorf("прочитать источник %d: %w", id, err)
	}
	return s, nil
}

func (r *SQLite) ListByKind(ctx context.Context, kind domain.Kind) ([]domain.Source, error) {
	list := []domain.Source{}
	if err := r.db.SelectContext(ctx, &list, `SELECT id, kind, title, config, created_at FROM sources
		WHERE kind = ? ORDER BY title, id`, kind); err != nil {
		return nil, fmt.Errorf("список источников: %w", err)
	}
	return list, nil
}
```

- [ ] **Step 4: Запустить — должен пройти**

Run: `go test ./internal/sources/...`
Expected: PASS.

- [ ] **Step 5: Проверка**

Run: `go vet ./... && go test ./...`
Expected: без ошибок.

---

### Task 3: Сообщения Telegram — модель и хранение

**Files:**
- Create: `internal/telegram/domain/telegram.go`, Test: `internal/telegram/domain/telegram_test.go`
- Create: `internal/telegram/repository/messages.go`, Test: `internal/telegram/repository/messages_test.go`

**Interfaces:**
- Consumes: таблицы `tg_messages`, `tg_media` (Task 1), `storetest.AddSource`
- Produces:
  - `domain.PeerType` (`PeerChannel "channel"`, `PeerChat "chat"`), `domain.Peer{Type, ID, AccessHash}` (JSON `type/id/access_hash`), `(Peer).Key() string`, `domain.ParsePeerKey(string) (Peer, error)`, `domain.ErrBadPeerKey`
  - `domain.Group{Peer, Title}`, `domain.MediaPhoto/MediaDocument`, `domain.Media{Kind, FileID, MimeType, FileName, Size}`, `domain.Message{ID int; Date int64; AuthorID int64; AuthorName, Text string; ReplyTo, ThreadTop int; FwdFrom string; EditDate int64; Media []Media; Raw string}`, `domain.Account{Authorized bool; Name string}`
  - ошибки `domain.ErrNotConfigured`, `ErrNotAuthorized`, `ErrPasswordNeeded`, `ErrBadPhone`, `ErrBadCode`, `ErrCodeExpired`, `ErrBadPassword`; `*domain.FloodWaitError{Wait time.Duration}`
  - `repository.ViaAccount/ViaBot`, `repository.Repository` — `Save(ctx, sourceID int64, msgs []domain.Message, via string, at int64) error`, `Bounds(ctx, sourceID) (minID, maxID int, err error)`, `Counts(ctx) (map[int64]int, error)` (без удалённых), `LiveIDsAfter(ctx, sourceID int64, afterID, limit int) ([]int, error)`, `MarkDeleted(ctx, sourceID int64, ids []int) error`; `repository.NewSQLite(db) *SQLite`

- [ ] **Step 1: Написать падающие тесты**

`internal/telegram/domain/telegram_test.go`:
```go
// Файл: internal/telegram/domain/telegram_test.go
package domain

import (
	"errors"
	"testing"
)

func TestPeerKeyRoundTrip(t *testing.T) {
	p := Peer{Type: PeerChannel, ID: 1234567890, AccessHash: -987654321}
	got, err := ParsePeerKey(p.Key())
	if err != nil || got != p {
		t.Fatalf("got %+v err %v", got, err)
	}
	for _, bad := range []string{"", "channel:1", "user:1:2", "chat:x:0", "chat:1:y"} {
		if _, err := ParsePeerKey(bad); !errors.Is(err, ErrBadPeerKey) {
			t.Fatalf("%q: %v", bad, err)
		}
	}
}
```

`internal/telegram/repository/messages_test.go`:
```go
// Файл: internal/telegram/repository/messages_test.go
package repository

import (
	"context"
	"testing"

	"github.com/jmoiron/sqlx"

	"pylesos/internal/store/storetest"
	"pylesos/internal/telegram/domain"
)

func msg(id int, text string) domain.Message {
	return domain.Message{ID: id, Date: int64(1000 + id), AuthorID: 7, AuthorName: "Тестовый Автор", Text: text}
}

func textOf(t *testing.T, db *sqlx.DB, src int64, id int) string {
	t.Helper()
	var s string
	if err := db.Get(&s, `SELECT text FROM tg_messages WHERE source_id = ? AND msg_id = ?`, src, id); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestSaveBoundsCountsAndMedia(t *testing.T) {
	ctx := context.Background()
	db := storetest.Open(t)
	src := storetest.AddSource(t, db, "Группа")
	repo := NewSQLite(db)
	photo := msg(3, "Фото документа")
	photo.Media = []domain.Media{{Kind: domain.MediaPhoto, FileID: 55, MimeType: "image/jpeg"}}
	if err := repo.Save(ctx, src, []domain.Message{msg(5, "пять"), photo, msg(4, "четыре")}, ViaAccount, 2000); err != nil {
		t.Fatal(err)
	}
	if err := repo.Save(ctx, src, []domain.Message{photo}, ViaAccount, 2001); err != nil {
		t.Fatalf("повторная запись с вложением: %v", err)
	}
	minID, maxID, err := repo.Bounds(ctx, src)
	if err != nil || minID != 3 || maxID != 5 {
		t.Fatalf("границы %d..%d %v", minID, maxID, err)
	}
	counts, err := repo.Counts(ctx)
	if err != nil || counts[src] != 3 {
		t.Fatalf("число сообщений: %v %v", counts, err)
	}
	var media int
	if err := db.Get(&media, `SELECT COUNT(*) FROM tg_media WHERE source_id = ? AND status = 'pending'`, src); err != nil || media != 1 {
		t.Fatalf("вложений %d %v", media, err)
	}
	if lo, hi, err := repo.Bounds(ctx, 999); err != nil || lo != 0 || hi != 0 {
		t.Fatalf("пустая группа: %d..%d %v", lo, hi, err)
	}
}

func TestResaveDoesNotEraseAndOlderEditDoesNotOverwrite(t *testing.T) {
	ctx := context.Background()
	db := storetest.Open(t)
	src := storetest.AddSource(t, db, "Группа")
	repo := NewSQLite(db)
	edited := msg(1, "Исправленный текст")
	edited.EditDate = 5000
	if err := repo.Save(ctx, src, []domain.Message{edited}, ViaAccount, 1); err != nil {
		t.Fatal(err)
	}
	stale := msg(1, "Старый текст") // пришла версия до правки
	if err := repo.Save(ctx, src, []domain.Message{stale}, ViaAccount, 2); err != nil {
		t.Fatal(err)
	}
	if got := textOf(t, db, src, 1); got != "Исправленный текст" {
		t.Fatalf("старая версия затёрла правку: %q", got)
	}
	empty := domain.Message{ID: 1, Date: 1001, EditDate: 6000} // без автора и текста
	if err := repo.Save(ctx, src, []domain.Message{empty}, ViaAccount, 3); err != nil {
		t.Fatal(err)
	}
	var author string
	if err := db.Get(&author, `SELECT author_name FROM tg_messages WHERE source_id = ? AND msg_id = 1`, src); err != nil {
		t.Fatal(err)
	}
	if author != "Тестовый Автор" || textOf(t, db, src, 1) != "Исправленный текст" {
		t.Fatalf("пустые поля затёрли сохранённое: автор %q, текст %q", author, textOf(t, db, src, 1))
	}
}

func TestMarkDeletedAndResaveRestores(t *testing.T) {
	ctx := context.Background()
	db := storetest.Open(t)
	src := storetest.AddSource(t, db, "Группа")
	repo := NewSQLite(db)
	if err := repo.Save(ctx, src, []domain.Message{msg(1, "а"), msg(2, "б"), msg(3, "в")}, ViaAccount, 1); err != nil {
		t.Fatal(err)
	}
	if err := repo.MarkDeleted(ctx, src, []int{2}); err != nil {
		t.Fatal(err)
	}
	if err := repo.MarkDeleted(ctx, src, nil); err != nil {
		t.Fatalf("пустой список: %v", err)
	}
	ids, err := repo.LiveIDsAfter(ctx, src, 0, 10)
	if err != nil || len(ids) != 2 || ids[0] != 1 || ids[1] != 3 {
		t.Fatalf("живые: %v %v", ids, err)
	}
	if ids, err := repo.LiveIDsAfter(ctx, src, 1, 1); err != nil || len(ids) != 1 || ids[0] != 3 {
		t.Fatalf("после 1, по одному: %v %v", ids, err)
	}
	counts, err := repo.Counts(ctx)
	if err != nil || counts[src] != 2 {
		t.Fatalf("удалённое посчитано: %v %v", counts, err)
	}
	if err := repo.Save(ctx, src, []domain.Message{msg(2, "б")}, ViaAccount, 2); err != nil {
		t.Fatal(err)
	}
	if counts, err := repo.Counts(ctx); err != nil || counts[src] != 3 {
		t.Fatalf("снова загруженное не восстановлено: %v %v", counts, err)
	}
}
```

- [ ] **Step 2: Запустить — должен упасть**

Run: `go test ./internal/telegram/...`
Expected: FAIL — пакеты/символы не определены.

- [ ] **Step 3: Реализовать**

`internal/telegram/domain/telegram.go`:
```go
// Файл: internal/telegram/domain/telegram.go
// Package domain — Telegram: группы, сообщения, вложения, ошибки входа.
package domain

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type PeerType string

const (
	PeerChannel PeerType = "channel" // супергруппа
	PeerChat    PeerType = "chat"    // обычная группа
)

// Peer — группа для API Telegram; JSON хранится в sources.config.
type Peer struct {
	Type       PeerType `json:"type"`
	ID         int64    `json:"id"`
	AccessHash int64    `json:"access_hash"`
}

// Key — ключ группы для экранов: «channel:ID:HASH».
func (p Peer) Key() string { return fmt.Sprintf("%s:%d:%d", p.Type, p.ID, p.AccessHash) }

var ErrBadPeerKey = errors.New("неверный ключ группы")

func ParsePeerKey(key string) (Peer, error) {
	parts := strings.Split(key, ":")
	if len(parts) != 3 || (parts[0] != string(PeerChannel) && parts[0] != string(PeerChat)) {
		return Peer{}, ErrBadPeerKey
	}
	id, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return Peer{}, ErrBadPeerKey
	}
	hash, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return Peer{}, ErrBadPeerKey
	}
	return Peer{Type: PeerType(parts[0]), ID: id, AccessHash: hash}, nil
}

type Group struct {
	Peer  Peer
	Title string
}

const (
	MediaPhoto    = "photo"
	MediaDocument = "document"
)

// Media — сведения о вложении; сам файл скачивается на разборе (П12).
type Media struct {
	Kind     string
	FileID   int64
	MimeType string
	FileName string
	Size     int64
}

type Message struct {
	ID         int
	Date       int64
	AuthorID   int64
	AuthorName string
	Text       string
	ReplyTo    int
	ThreadTop  int
	FwdFrom    string
	EditDate   int64
	Media      []Media
	Raw        string // сообщение Telegram в JSON
}

type Account struct {
	Authorized bool
	Name       string
}

var (
	ErrNotConfigured  = errors.New("не заданы api_id и api_hash")
	ErrNotAuthorized  = errors.New("вход в Telegram не выполнен")
	ErrPasswordNeeded = errors.New("нужен облачный пароль")
	ErrBadPhone       = errors.New("Telegram не принял номер")
	ErrBadCode        = errors.New("неверный код")
	ErrCodeExpired    = errors.New("код устарел")
	ErrBadPassword    = errors.New("неверный облачный пароль")
)

// FloodWaitError — Telegram просит подождать (FLOOD_WAIT).
type FloodWaitError struct{ Wait time.Duration }

func (e *FloodWaitError) Error() string { return fmt.Sprintf("FLOOD_WAIT %s", e.Wait) }
```

`internal/telegram/repository/messages.go`:
```go
// Файл: internal/telegram/repository/messages.go
// Package repository — сообщения Telegram и сведения о вложениях.
package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jmoiron/sqlx"

	"pylesos/internal/telegram/domain"
)

// Кто принёс сообщение (П6).
const (
	ViaAccount = "account"
	ViaBot     = "bot"
)

type Repository interface {
	Save(ctx context.Context, sourceID int64, msgs []domain.Message, via string, at int64) error
	Bounds(ctx context.Context, sourceID int64) (minID, maxID int, err error)
	Counts(ctx context.Context) (map[int64]int, error)
	LiveIDsAfter(ctx context.Context, sourceID int64, afterID, limit int) ([]int, error)
	MarkDeleted(ctx context.Context, sourceID int64, ids []int) error
}

// Повторная загрузка не затирает сохранённое пустым и старой версией правки (02-database.md).
const upsertMessage = `INSERT INTO tg_messages (source_id, msg_id, date, author_id, author_name, text, reply_to,
	thread_top, fwd_from, edit_date, via, raw, fetched_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (source_id, msg_id) DO UPDATE SET
	author_id   = COALESCE(NULLIF(excluded.author_id, 0), tg_messages.author_id),
	author_name = COALESCE(NULLIF(excluded.author_name, ''), tg_messages.author_name),
	text        = CASE WHEN excluded.edit_date >= tg_messages.edit_date
	                   THEN COALESCE(NULLIF(excluded.text, ''), tg_messages.text) ELSE tg_messages.text END,
	edit_date   = MAX(excluded.edit_date, tg_messages.edit_date),
	raw         = COALESCE(NULLIF(excluded.raw, ''), tg_messages.raw),
	deleted     = 0,
	fetched_at  = excluded.fetched_at`

const insertMedia = `INSERT INTO tg_media (source_id, msg_id, idx, kind, file_id, mime_type, file_name, size)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT (source_id, msg_id, idx) DO NOTHING`

type SQLite struct{ db *sqlx.DB }

func NewSQLite(db *sqlx.DB) *SQLite { return &SQLite{db: db} }

// Save сохраняет порцию сообщений одной транзакцией.
func (r *SQLite) Save(ctx context.Context, sourceID int64, msgs []domain.Message, via string, at int64) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("начать запись сообщений: %w", err)
	}
	for _, m := range msgs {
		if err := saveOne(ctx, tx, sourceID, m, via, at); err != nil {
			return errors.Join(err, tx.Rollback())
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("сохранить сообщения: %w", err)
	}
	return nil
}

func saveOne(ctx context.Context, tx *sqlx.Tx, sourceID int64, m domain.Message, via string, at int64) error {
	if _, err := tx.ExecContext(ctx, upsertMessage, sourceID, m.ID, m.Date, m.AuthorID, m.AuthorName, m.Text,
		m.ReplyTo, m.ThreadTop, m.FwdFrom, m.EditDate, via, m.Raw, at); err != nil {
		return fmt.Errorf("сообщение %d: %w", m.ID, err)
	}
	for i, md := range m.Media {
		if _, err := tx.ExecContext(ctx, insertMedia, sourceID, m.ID, i, md.Kind, md.FileID, md.MimeType, md.FileName, md.Size); err != nil {
			return fmt.Errorf("вложение %d/%d: %w", m.ID, i, err)
		}
	}
	return nil
}

// Bounds — наименьший и наибольший номер сохранённых сообщений группы (0, 0 — сообщений нет).
func (r *SQLite) Bounds(ctx context.Context, sourceID int64) (int, int, error) {
	var b struct {
		Min int `db:"min_id"`
		Max int `db:"max_id"`
	}
	if err := r.db.GetContext(ctx, &b, `SELECT COALESCE(MIN(msg_id), 0) AS min_id, COALESCE(MAX(msg_id), 0) AS max_id
		FROM tg_messages WHERE source_id = ?`, sourceID); err != nil {
		return 0, 0, fmt.Errorf("границы сообщений: %w", err)
	}
	return b.Min, b.Max, nil
}

// Counts — число неудалённых сообщений по источникам.
func (r *SQLite) Counts(ctx context.Context) (map[int64]int, error) {
	var rows []struct {
		SourceID int64 `db:"source_id"`
		N        int   `db:"n"`
	}
	if err := r.db.SelectContext(ctx, &rows, `SELECT source_id, COUNT(*) AS n FROM tg_messages
		WHERE deleted = 0 GROUP BY source_id`); err != nil {
		return nil, fmt.Errorf("число сообщений: %w", err)
	}
	out := make(map[int64]int, len(rows))
	for _, x := range rows {
		out[x.SourceID] = x.N
	}
	return out, nil
}

func (r *SQLite) LiveIDsAfter(ctx context.Context, sourceID int64, afterID, limit int) ([]int, error) {
	ids := []int{}
	if err := r.db.SelectContext(ctx, &ids, `SELECT msg_id FROM tg_messages
		WHERE source_id = ? AND deleted = 0 AND msg_id > ? ORDER BY msg_id LIMIT ?`, sourceID, afterID, limit); err != nil {
		return nil, fmt.Errorf("номера сообщений: %w", err)
	}
	return ids, nil
}

func (r *SQLite) MarkDeleted(ctx context.Context, sourceID int64, ids []int) error {
	if len(ids) == 0 {
		return nil
	}
	query, args, err := sqlx.In(`UPDATE tg_messages SET deleted = 1 WHERE source_id = ? AND msg_id IN (?)`, sourceID, ids)
	if err != nil {
		return fmt.Errorf("запрос удалённых: %w", err)
	}
	if _, err := r.db.ExecContext(ctx, r.db.Rebind(query), args...); err != nil {
		return fmt.Errorf("пометить удалённые: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: Запустить — должен пройти**

Run: `go test ./internal/telegram/...`
Expected: PASS.

- [ ] **Step 5: Проверка**

Run: `go vet ./... && go test ./...`
Expected: без ошибок.

---

### Task 4: Вход в Telegram и группы

**Files:**
- Create: `internal/telegram/service/ports.go`, `internal/telegram/service/keys.go`, `internal/telegram/service/validate.go`, `internal/telegram/service/login.go`, `internal/telegram/service/groups.go`
- Modify: `internal/texts/ru.go` (тексты Telegram — полный блок ниже)
- Test: `internal/telegram/service/fake_test.go`, `internal/telegram/service/login_test.go`, `internal/telegram/service/groups_test.go`

**Interfaces:**
- Consumes: `domain.*` (Task 3), `sourcesdomain.Source/Kind`, `sourcesrepo.NewSQLite` (Task 2, тесты), `jobsdomain.ErrSourceBusy` (M1), `texts.*`
- Produces:
  - `service.Gateway` — `Account(ctx) (domain.Account, error)`, `SendCode(ctx, phone) (string, error)`, `SignIn(ctx, phone, code, codeHash string) error`, `Password(ctx, password) error`, `Groups(ctx) ([]domain.Group, error)`, `History(ctx, p domain.Peer, offsetID, minID, limit int) ([]domain.Message, error)`, `Alive(ctx, p domain.Peer, ids []int) (map[int]bool, error)`, `Reset()`
  - `service.SecretStore` — `Get(ctx, key) ([]byte, bool, error)`, `Put(ctx, key string, value []byte) error`
  - `service.Keys{AppID int; AppHash string}`, `service.NewKeyStore(SecretStore) *KeyStore`, `(*KeyStore).Load(ctx) (Keys, bool, error)`, `(*KeyStore).Save(ctx, Keys) error`
  - поля `service.FieldAPIID "api_id"`, `FieldAPIHash "api_hash"`, `FieldPhone "phone"`, `FieldCode "code"`, `FieldPassword "password"`
  - `service.Step` (`StepKeys`, `StepPhone`, `StepCode`, `StepPassword`, `StepReady`), `service.LoginState{Step; Name; Phone}`, `service.NewLogin(Gateway, *KeyStore) *Login`, `(*Login).State(ctx) (LoginState, error)`, `(*Login).Submit(ctx, map[string]string) (map[string]string, error)`
  - `service.KindSync = "tg_sync"`, `service.SourceStore` (`Ensure`, `ListByKind`), `service.JobStarter` (`Start(ctx, sourceID int64, kind, params string) (int64, error)`), `service.ErrGroupGone`, `service.GroupView{Group domain.Group; Added bool}`, `service.NewGroups(Gateway, SourceStore, JobStarter, func() time.Time) *Groups`, `(*Groups).List(ctx, refresh bool) ([]GroupView, error)`, `(*Groups).Add(ctx, key string) error`, `(*Groups).Collect(ctx, sourceID int64) error`, `(*Groups).SyncAll(ctx) error`

- [ ] **Step 1: Тексты Telegram**

Дописать в `internal/texts/ru.go`:
```go
// Telegram (M2).
const (
	NavTelegram       = "Telegram"
	TgTitle           = "Telegram"
	TgKeysHint        = "Получите api_id и api_hash на my.telegram.org → «API development tools» и введите их здесь."
	TgPhoneHint       = "Введите номер телефона вашего аккаунта Telegram."
	TgCodeHint        = "Telegram прислал код в приложение (или SMS) на номер %s."
	TgPasswordHint    = "У аккаунта включена двухэтапная проверка — введите облачный пароль."
	TgAPIID           = "api_id"
	TgAPIHash         = "api_hash"
	TgPhone           = "Номер телефона"
	TgCode            = "Код из Telegram"
	TgPassword        = "Облачный пароль"
	TgNext            = "Далее"
	TgLoggedIn        = "Вход выполнен: %s"
	TgGroupsTitle     = "Ваши группы"
	TgGroupsEmpty     = "Групп не найдено."
	TgRefresh         = "Обновить список"
	TgAddGroup        = "Добавить"
	TgAdded           = "Добавлена"
	TgGroupAdded      = "Группа добавлена, сбор начат"
	TgSyncKind        = "Сбор Telegram"
	TgCollect         = "Собрать"
	TgMessagesCount   = "Сообщений: %d"
	TgLastSync        = "Последний сбор: %s"
	TgNeverSynced     = "Ещё не собиралась"
	TgNeedLogin       = "Войдите в Telegram на вкладке «Telegram»"
	TgFloodWait       = "Telegram попросил подождать %d с"
	TgProgressNew     = "Новые сообщения: загружено %d"
	TgProgressOld     = "История: загружено %d, дошли до сообщения №%d"
	TgProgressDeleted = "Проверка удалённых: проверено %d"
	TgProgressDone    = "Готово: загружено %d"
	HomeTelegramTitle = "Группы Telegram"
	ErrTgAPIID        = "api_id — это число, например 1234567"
	ErrTgAPIHash      = "api_hash — 32 символа: цифры и буквы a–f"
	ErrTgPhone        = "Введите номер в международном формате, например +77011234567"
	ErrTgBadPhone     = "Telegram не принял этот номер"
	ErrTgBadCode      = "Неверный код"
	ErrTgCodeExpired  = "Код устарел — введите номер ещё раз"
	ErrTgBadPassword  = "Неверный облачный пароль"
	ErrTgGroupGone    = "Группа не найдена — обновите список"
)
```

- [ ] **Step 2: Общая подставка шлюза и падающие тесты**

`internal/telegram/service/fake_test.go`:
```go
// Файл: internal/telegram/service/fake_test.go
package service

import (
	"context"
	"sort"
	"sync"
	"time"

	"pylesos/internal/telegram/domain"
)

// fakeGateway — Telegram в памяти: вход и одна группа с сообщениями.
type fakeGateway struct {
	mu           sync.Mutex
	authorized   bool
	name         string
	sentPhone    string
	sendErr      error
	codeErr      error
	passErr      error
	needPassword bool
	resets       int
	groups       []domain.Group
	msgs         map[int]domain.Message
	flood        time.Duration // одна FLOOD_WAIT на ближайший History
	historyErr   error
	minIDs       []int // minID каждого вызова History
}

func (f *fakeGateway) Account(context.Context) (domain.Account, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return domain.Account{Authorized: f.authorized, Name: f.name}, nil
}

func (f *fakeGateway) SendCode(_ context.Context, phone string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sentPhone = phone
	return "hash-1", f.sendErr
}

func (f *fakeGateway) SignIn(_ context.Context, _, _, hash string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch {
	case f.codeErr != nil:
		return f.codeErr
	case hash != "hash-1":
		return domain.ErrBadCode
	case f.needPassword:
		return domain.ErrPasswordNeeded
	}
	f.authorized = true
	return nil
}

func (f *fakeGateway) Password(context.Context, string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.passErr != nil {
		return f.passErr
	}
	f.authorized = true
	return nil
}

func (f *fakeGateway) Groups(context.Context) ([]domain.Group, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]domain.Group{}, f.groups...), nil
}

func (f *fakeGateway) History(_ context.Context, _ domain.Peer, offsetID, minID, limit int) ([]domain.Message, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.flood > 0 {
		wait := f.flood
		f.flood = 0
		return nil, &domain.FloodWaitError{Wait: wait}
	}
	if f.historyErr != nil {
		return nil, f.historyErr
	}
	f.minIDs = append(f.minIDs, minID)
	ids := []int{}
	for id := range f.msgs {
		if (offsetID == 0 || id < offsetID) && id > minID {
			ids = append(ids, id)
		}
	}
	sort.Sort(sort.Reverse(sort.IntSlice(ids)))
	out := []domain.Message{}
	for _, id := range ids {
		if len(out) == limit {
			break
		}
		out = append(out, f.msgs[id])
	}
	return out, nil
}

func (f *fakeGateway) Alive(_ context.Context, _ domain.Peer, ids []int) (map[int]bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	alive := map[int]bool{}
	for _, id := range ids {
		if _, ok := f.msgs[id]; ok {
			alive[id] = true
		}
	}
	return alive, nil
}

func (f *fakeGateway) Reset() {
	f.mu.Lock()
	f.resets++
	f.mu.Unlock()
}

// fakeSecrets — секреты в памяти.
type fakeSecrets map[string][]byte

func (s fakeSecrets) Get(_ context.Context, key string) ([]byte, bool, error) {
	v, ok := s[key]
	return v, ok, nil
}

func (s fakeSecrets) Put(_ context.Context, key string, value []byte) error {
	s[key] = value
	return nil
}
```

`internal/telegram/service/login_test.go`:
```go
// Файл: internal/telegram/service/login_test.go
package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"pylesos/internal/telegram/domain"
	"pylesos/internal/texts"
)

func newLogin(t *testing.T, gw *fakeGateway, withKeys bool) *Login {
	t.Helper()
	keys := NewKeyStore(fakeSecrets{})
	if withKeys {
		if err := keys.Save(context.Background(), Keys{AppID: 1, AppHash: "0123456789abcdef0123456789abcdef"}); err != nil {
			t.Fatal(err)
		}
	}
	return NewLogin(gw, keys)
}

func state(t *testing.T, l *Login) LoginState {
	t.Helper()
	st, err := l.State(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func submit(t *testing.T, l *Login, values map[string]string) map[string]string {
	t.Helper()
	problems, err := l.Submit(context.Background(), values)
	if err != nil {
		t.Fatal(err)
	}
	return problems
}

func TestKeysStepValidatesAndSaves(t *testing.T) {
	gw := &fakeGateway{}
	l := newLogin(t, gw, false)
	if st := state(t, l); st.Step != StepKeys {
		t.Fatalf("шаг %q", st.Step)
	}
	problems := submit(t, l, map[string]string{FieldAPIID: "abc", FieldAPIHash: "короткий"})
	if problems[FieldAPIID] != texts.ErrTgAPIID || problems[FieldAPIHash] != texts.ErrTgAPIHash {
		t.Fatalf("ошибки: %v", problems)
	}
	problems = submit(t, l, map[string]string{FieldAPIID: " 1234567 ", FieldAPIHash: "0123456789ABCDEF0123456789abcdef"})
	if len(problems) != 0 || gw.resets != 1 {
		t.Fatalf("ошибки %v, перезапусков %d", problems, gw.resets)
	}
	if st := state(t, l); st.Step != StepPhone {
		t.Fatalf("после ключей шаг %q", st.Step)
	}
}

func TestPhoneCodePasswordFlow(t *testing.T) {
	gw := &fakeGateway{needPassword: true, name: "Тестовый Пользователь"}
	l := newLogin(t, gw, true)
	if p := submit(t, l, map[string]string{FieldPhone: "8 (701) 123-45-67"}); len(p) != 0 || gw.sentPhone != "+77011234567" {
		t.Fatalf("ошибки %v, номер %q", p, gw.sentPhone)
	}
	if st := state(t, l); st.Step != StepCode || st.Phone != "+77011234567" {
		t.Fatalf("шаг кода: %+v", st)
	}
	gw.codeErr = domain.ErrBadCode
	if p := submit(t, l, map[string]string{FieldCode: "11111"}); p[FieldCode] != texts.ErrTgBadCode {
		t.Fatalf("неверный код: %v", p)
	}
	gw.codeErr = nil
	if p := submit(t, l, map[string]string{FieldCode: "22222"}); len(p) != 0 {
		t.Fatalf("код: %v", p)
	}
	if st := state(t, l); st.Step != StepPassword {
		t.Fatalf("шаг пароля: %+v", st)
	}
	gw.passErr = domain.ErrBadPassword
	if p := submit(t, l, map[string]string{FieldPassword: "не тот"}); p[FieldPassword] != texts.ErrTgBadPassword {
		t.Fatalf("неверный пароль: %v", p)
	}
	gw.passErr = nil
	if p := submit(t, l, map[string]string{FieldPassword: "верный"}); len(p) != 0 {
		t.Fatalf("пароль: %v", p)
	}
	if st := state(t, l); st.Step != StepReady || st.Name != "Тестовый Пользователь" {
		t.Fatalf("после входа: %+v", st)
	}
}

func TestExpiredCodeReturnsToPhone(t *testing.T) {
	gw := &fakeGateway{}
	l := newLogin(t, gw, true)
	submit(t, l, map[string]string{FieldPhone: "+77011234567"})
	gw.codeErr = domain.ErrCodeExpired
	if p := submit(t, l, map[string]string{FieldCode: "33333"}); p[FieldPhone] != texts.ErrTgCodeExpired {
		t.Fatalf("устаревший код: %v", p)
	}
	if st := state(t, l); st.Step != StepPhone {
		t.Fatalf("после устаревшего кода шаг %q", st.Step)
	}
}

func TestPhoneProblems(t *testing.T) {
	gw := &fakeGateway{}
	l := newLogin(t, gw, true)
	if p := submit(t, l, map[string]string{FieldPhone: "позвоните мне"}); p[FieldPhone] != texts.ErrTgPhone || gw.sentPhone != "" {
		t.Fatalf("мусор вместо номера: %v, отправлено %q", p, gw.sentPhone)
	}
	gw.sendErr = &domain.FloodWaitError{Wait: 30 * time.Second}
	if p := submit(t, l, map[string]string{FieldPhone: "+77011234567"}); p[FieldPhone] != fmt.Sprintf(texts.TgFloodWait, 30) {
		t.Fatalf("FLOOD_WAIT: %v", p)
	}
	gw.sendErr = domain.ErrBadPhone
	if p := submit(t, l, map[string]string{FieldPhone: "+77011234567"}); p[FieldPhone] != texts.ErrTgBadPhone {
		t.Fatalf("номер не принят: %v", p)
	}
}
```

`internal/telegram/service/groups_test.go`:
```go
// Файл: internal/telegram/service/groups_test.go
package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	jobsdomain "pylesos/internal/jobs/domain"
	sourcesdomain "pylesos/internal/sources/domain"
	sourcesrepo "pylesos/internal/sources/repository"
	"pylesos/internal/store/storetest"
	"pylesos/internal/telegram/domain"
)

type fakeStarter struct {
	mu      sync.Mutex
	started []int64
	err     error
}

func (f *fakeStarter) Start(_ context.Context, sourceID int64, kind, _ string) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if kind != KindSync {
		return 0, errors.New("не тот вид задания: " + kind)
	}
	f.started = append(f.started, sourceID)
	return int64(len(f.started)), f.err
}

var testGroup = domain.Group{Peer: domain.Peer{Type: domain.PeerChannel, ID: 10, AccessHash: 20}, Title: "Тестовая группа"}

func newGroups(t *testing.T, gw *fakeGateway) (*Groups, *sourcesrepo.SQLite, *fakeStarter) {
	t.Helper()
	repo := sourcesrepo.NewSQLite(storetest.Open(t))
	starter := &fakeStarter{}
	now := func() time.Time { return time.Unix(1000, 0) }
	return NewGroups(gw, repo, starter, now), repo, starter
}

func TestAddGroupCreatesSourceOnceAndStartsSync(t *testing.T) {
	ctx := context.Background()
	g, repo, starter := newGroups(t, &fakeGateway{authorized: true, groups: []domain.Group{testGroup}})
	views, err := g.List(ctx, false)
	if err != nil || len(views) != 1 || views[0].Added {
		t.Fatalf("список: %+v %v", views, err)
	}
	if err := g.Add(ctx, testGroup.Peer.Key()); err != nil {
		t.Fatal(err)
	}
	starter.err = jobsdomain.ErrSourceBusy // второй клик, пока идёт сбор
	if err := g.Add(ctx, testGroup.Peer.Key()); err != nil {
		t.Fatalf("повторное добавление: %v", err)
	}
	list, err := repo.ListByKind(ctx, sourcesdomain.KindTelegramGroup)
	if err != nil || len(list) != 1 || list[0].Title != "Тестовая группа" {
		t.Fatalf("источники: %+v %v", list, err)
	}
	if len(starter.started) != 2 || starter.started[0] != list[0].ID {
		t.Fatalf("запуски сбора: %v", starter.started)
	}
	views, err = g.List(ctx, false)
	if err != nil || !views[0].Added {
		t.Fatalf("после добавления: %+v %v", views, err)
	}
	if err := g.Collect(ctx, list[0].ID); !errors.Is(err, jobsdomain.ErrSourceBusy) {
		t.Fatalf("«Собрать» при идущем сборе: %v", err)
	}
}

func TestAddUnknownGroup(t *testing.T) {
	g, _, _ := newGroups(t, &fakeGateway{authorized: true})
	for _, key := range []string{"мусор", testGroup.Peer.Key()} {
		if err := g.Add(context.Background(), key); !errors.Is(err, ErrGroupGone) {
			t.Fatalf("%q: %v", key, err)
		}
	}
}

func TestSyncAllOnlyWhenAuthorized(t *testing.T) {
	ctx := context.Background()
	gw := &fakeGateway{authorized: true, groups: []domain.Group{testGroup}}
	g, _, starter := newGroups(t, gw)
	if _, err := g.List(ctx, false); err != nil {
		t.Fatal(err)
	}
	if err := g.Add(ctx, testGroup.Peer.Key()); err != nil {
		t.Fatal(err)
	}
	gw.authorized = false
	if err := g.SyncAll(ctx); err != nil || len(starter.started) != 1 {
		t.Fatalf("без входа: %v, запусков %d", err, len(starter.started))
	}
	gw.authorized = true
	starter.err = jobsdomain.ErrSourceBusy // задание уже продолжено после перезапуска
	if err := g.SyncAll(ctx); err != nil || len(starter.started) != 2 {
		t.Fatalf("с входом: %v, запусков %d", err, len(starter.started))
	}
}
```

- [ ] **Step 3: Запустить — должен упасть**

Run: `go test ./internal/telegram/service/`
Expected: FAIL — символы пакета не определены.

- [ ] **Step 4: Реализовать**

`internal/telegram/service/ports.go`:
```go
// Файл: internal/telegram/service/ports.go
// Package service — вход в Telegram, группы и сбор истории (П6; спецификация, раздел 3).
package service

import (
	"context"

	"pylesos/internal/telegram/domain"
)

// Gateway — Telegram (реализация — gotdgw). Ошибки — из domain: ErrNotAuthorized, ErrBadCode, *FloodWaitError…
type Gateway interface {
	Account(ctx context.Context) (domain.Account, error)
	SendCode(ctx context.Context, phone string) (codeHash string, err error)
	SignIn(ctx context.Context, phone, code, codeHash string) error
	Password(ctx context.Context, password string) error
	Groups(ctx context.Context) ([]domain.Group, error)
	// History — сообщения от новых к старым: id < offsetID (0 — с самого нового) и id > minID.
	History(ctx context.Context, p domain.Peer, offsetID, minID, limit int) ([]domain.Message, error)
	// Alive — какие из ids ещё существуют в группе.
	Alive(ctx context.Context, p domain.Peer, ids []int) (map[int]bool, error)
	// Reset разрывает соединение (сменились ключи API).
	Reset()
}

// SecretStore — шифрованные секреты (internal/secrets).
type SecretStore interface {
	Get(ctx context.Context, key string) ([]byte, bool, error)
	Put(ctx context.Context, key string, value []byte) error
}
```

`internal/telegram/service/keys.go`:
```go
// Файл: internal/telegram/service/keys.go
package service

import (
	"context"
	"encoding/json"
	"fmt"
)

const secretAPIKeys = "telegram_api"

// Keys — api_id и api_hash приложения Telegram (вводит пользователь, П19).
type Keys struct {
	AppID   int    `json:"api_id"`
	AppHash string `json:"api_hash"`
}

type KeyStore struct{ secrets SecretStore }

func NewKeyStore(s SecretStore) *KeyStore { return &KeyStore{secrets: s} }

// Load — сохранённые ключи; ok=false — ещё не введены.
func (k *KeyStore) Load(ctx context.Context) (Keys, bool, error) {
	data, ok, err := k.secrets.Get(ctx, secretAPIKeys)
	if err != nil || !ok {
		return Keys{}, false, err
	}
	var keys Keys
	if err := json.Unmarshal(data, &keys); err != nil {
		return Keys{}, false, fmt.Errorf("ключи API Telegram: %w", err)
	}
	return keys, true, nil
}

func (k *KeyStore) Save(ctx context.Context, keys Keys) error {
	data, err := json.Marshal(keys)
	if err != nil {
		return fmt.Errorf("ключи API Telegram: %w", err)
	}
	return k.secrets.Put(ctx, secretAPIKeys, data)
}
```

`internal/telegram/service/validate.go`:
```go
// Файл: internal/telegram/service/validate.go
package service

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"pylesos/internal/telegram/domain"
	"pylesos/internal/texts"
)

// Поля формы входа.
const (
	FieldAPIID    = "api_id"
	FieldAPIHash  = "api_hash"
	FieldPhone    = "phone"
	FieldCode     = "code"
	FieldPassword = "password"
)

func validateKeys(values map[string]string) (Keys, map[string]string) {
	problems := map[string]string{}
	id, err := strconv.Atoi(strings.TrimSpace(values[FieldAPIID]))
	if err != nil || id <= 0 {
		problems[FieldAPIID] = texts.ErrTgAPIID
	}
	hash := strings.ToLower(strings.TrimSpace(values[FieldAPIHash]))
	if !isHex32(hash) {
		problems[FieldAPIHash] = texts.ErrTgAPIHash
	}
	return Keys{AppID: id, AppHash: hash}, problems
}

func isHex32(s string) bool {
	if len(s) != 32 {
		return false
	}
	for _, r := range s {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
			return false
		}
	}
	return true
}

// normalizePhone: цифры с «+»; «8XXXXXXXXXX» (Казахстан, Россия) → «+7XXXXXXXXXX».
func normalizePhone(raw string) (string, bool) {
	var b strings.Builder
	for i, r := range strings.TrimSpace(raw) {
		switch {
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '+' && i == 0, r == ' ', r == '-', r == '(', r == ')':
		default:
			return "", false
		}
	}
	digits := b.String()
	if len(digits) == 11 && digits[0] == '8' {
		digits = "7" + digits[1:]
	}
	if len(digits) < 10 || len(digits) > 15 {
		return "", false
	}
	return "+" + digits, true
}

// problemOf — ошибка Telegram, которую пользователь исправит сам, готовым текстом; "" — не такая.
func problemOf(err error) string {
	var flood *domain.FloodWaitError
	switch {
	case err == nil:
		return ""
	case errors.As(err, &flood):
		return fmt.Sprintf(texts.TgFloodWait, int(flood.Wait.Seconds()))
	case errors.Is(err, domain.ErrBadPhone):
		return texts.ErrTgBadPhone
	case errors.Is(err, domain.ErrBadCode):
		return texts.ErrTgBadCode
	case errors.Is(err, domain.ErrCodeExpired):
		return texts.ErrTgCodeExpired
	case errors.Is(err, domain.ErrBadPassword):
		return texts.ErrTgBadPassword
	default:
		return ""
	}
}
```

`internal/telegram/service/login.go`:
```go
// Файл: internal/telegram/service/login.go
package service

import (
	"context"
	"errors"
	"strings"
	"sync"

	"pylesos/internal/telegram/domain"
	"pylesos/internal/texts"
)

type Step string

const (
	StepKeys     Step = "keys"
	StepPhone    Step = "phone"
	StepCode     Step = "code"
	StepPassword Step = "password"
	StepReady    Step = "ready"
)

type LoginState struct {
	Step  Step
	Name  string // имя аккаунта, когда вход выполнен
	Phone string // номер, на который отправлен код
}

// Login ведёт вход по шагам: ключи API → номер → код → облачный пароль.
type Login struct {
	gw   Gateway
	keys *KeyStore

	mu    sync.Mutex
	step  Step // StepCode или StepPassword, пока идёт вход; иначе ""
	phone string
	hash  string
}

func NewLogin(gw Gateway, keys *KeyStore) *Login { return &Login{gw: gw, keys: keys} }

func (l *Login) State(ctx context.Context) (LoginState, error) {
	if _, ok, err := l.keys.Load(ctx); err != nil || !ok {
		return LoginState{Step: StepKeys}, err
	}
	l.mu.Lock()
	pending := LoginState{Step: l.step, Phone: l.phone}
	l.mu.Unlock()
	if pending.Step != "" {
		return pending, nil
	}
	acc, err := l.gw.Account(ctx)
	if err != nil {
		return LoginState{}, err
	}
	if acc.Authorized {
		return LoginState{Step: StepReady, Name: acc.Name}, nil
	}
	return LoginState{Step: StepPhone}, nil
}

// Submit выполняет текущий шаг; ошибки ввода — по полям, готовым текстом.
func (l *Login) Submit(ctx context.Context, values map[string]string) (map[string]string, error) {
	st, err := l.State(ctx)
	if err != nil {
		return nil, err
	}
	switch st.Step {
	case StepKeys:
		return l.submitKeys(ctx, values)
	case StepPhone:
		return l.submitPhone(ctx, values[FieldPhone])
	case StepCode:
		return l.submitCode(ctx, st.Phone, values[FieldCode])
	case StepPassword:
		return l.submitPassword(ctx, values[FieldPassword])
	default:
		return map[string]string{}, nil
	}
}

func (l *Login) submitKeys(ctx context.Context, values map[string]string) (map[string]string, error) {
	keys, problems := validateKeys(values)
	if len(problems) > 0 {
		return problems, nil
	}
	if err := l.keys.Save(ctx, keys); err != nil {
		return nil, err
	}
	l.gw.Reset()
	return map[string]string{}, nil
}

func (l *Login) submitPhone(ctx context.Context, raw string) (map[string]string, error) {
	phone, ok := normalizePhone(raw)
	if !ok {
		return map[string]string{FieldPhone: texts.ErrTgPhone}, nil
	}
	hash, err := l.gw.SendCode(ctx, phone)
	if p := problemOf(err); p != "" {
		return map[string]string{FieldPhone: p}, nil
	}
	if err != nil {
		return nil, err
	}
	l.set(StepCode, phone, hash)
	return map[string]string{}, nil
}

func (l *Login) submitCode(ctx context.Context, phone, raw string) (map[string]string, error) {
	code := strings.TrimSpace(raw)
	if code == "" {
		return map[string]string{FieldCode: texts.ErrTgBadCode}, nil
	}
	l.mu.Lock()
	hash := l.hash
	l.mu.Unlock()
	err := l.gw.SignIn(ctx, phone, code, hash)
	switch {
	case errors.Is(err, domain.ErrPasswordNeeded):
		l.set(StepPassword, phone, hash)
		return map[string]string{}, nil
	case errors.Is(err, domain.ErrCodeExpired):
		l.set("", "", "")
		return map[string]string{FieldPhone: texts.ErrTgCodeExpired}, nil
	}
	if p := problemOf(err); p != "" {
		return map[string]string{FieldCode: p}, nil
	}
	if err != nil {
		return nil, err
	}
	l.set("", "", "")
	return map[string]string{}, nil
}

func (l *Login) submitPassword(ctx context.Context, password string) (map[string]string, error) {
	err := l.gw.Password(ctx, password)
	if p := problemOf(err); p != "" {
		return map[string]string{FieldPassword: p}, nil
	}
	if err != nil {
		return nil, err
	}
	l.set("", "", "")
	return map[string]string{}, nil
}

func (l *Login) set(step Step, phone, hash string) {
	l.mu.Lock()
	l.step, l.phone, l.hash = step, phone, hash
	l.mu.Unlock()
}
```

`internal/telegram/service/groups.go`:
```go
// Файл: internal/telegram/service/groups.go
package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	jobsdomain "pylesos/internal/jobs/domain"
	sourcesdomain "pylesos/internal/sources/domain"
	"pylesos/internal/telegram/domain"
)

// KindSync — вид задания «сбор Telegram».
const KindSync = "tg_sync"

type SourceStore interface {
	Ensure(ctx context.Context, s sourcesdomain.Source) (int64, bool, error)
	ListByKind(ctx context.Context, kind sourcesdomain.Kind) ([]sourcesdomain.Source, error)
}

type JobStarter interface {
	Start(ctx context.Context, sourceID int64, kind, params string) (int64, error)
}

var ErrGroupGone = errors.New("группа не найдена в списке")

type GroupView struct {
	Group domain.Group
	Added bool // уже источник Пылесоса
}

type Groups struct {
	gw      Gateway
	sources SourceStore
	jobs    JobStarter
	now     func() time.Time

	mu    sync.Mutex
	cache []domain.Group
}

func NewGroups(gw Gateway, sources SourceStore, jobs JobStarter, now func() time.Time) *Groups {
	return &Groups{gw: gw, sources: sources, jobs: jobs, now: now}
}

// List — группы аккаунта с отметкой «добавлена». Список из Telegram кэшируется; refresh — перечитать.
func (g *Groups) List(ctx context.Context, refresh bool) ([]GroupView, error) {
	groups, err := g.groups(ctx, refresh)
	if err != nil {
		return nil, err
	}
	added, err := g.addedConfigs(ctx)
	if err != nil {
		return nil, err
	}
	views := make([]GroupView, 0, len(groups))
	for _, gr := range groups {
		cfg, err := configOf(gr.Peer)
		if err != nil {
			return nil, err
		}
		views = append(views, GroupView{Group: gr, Added: added[cfg]})
	}
	return views, nil
}

// Add делает группу источником и сразу запускает сбор; идущий сбор — не ошибка.
func (g *Groups) Add(ctx context.Context, key string) error {
	peer, err := domain.ParsePeerKey(key)
	if err != nil {
		return ErrGroupGone
	}
	group, ok := g.find(peer)
	if !ok {
		return ErrGroupGone
	}
	cfg, err := configOf(peer)
	if err != nil {
		return err
	}
	id, _, err := g.sources.Ensure(ctx, sourcesdomain.Source{Kind: sourcesdomain.KindTelegramGroup,
		Title: group.Title, Config: cfg, CreatedAt: g.now().Unix()})
	if err != nil {
		return err
	}
	return ignoreBusy(g.Collect(ctx, id))
}

// Collect — кнопка «Собрать»: занятый источник — ошибка ErrSourceBusy для экрана.
func (g *Groups) Collect(ctx context.Context, sourceID int64) error {
	_, err := g.jobs.Start(ctx, sourceID, KindSync, "{}")
	return err
}

// SyncAll при запуске программы догружает все группы, если вход выполнен (спецификация, раздел 3, п. 4).
func (g *Groups) SyncAll(ctx context.Context) error {
	acc, err := g.gw.Account(ctx)
	if err != nil || !acc.Authorized {
		return err
	}
	list, err := g.sources.ListByKind(ctx, sourcesdomain.KindTelegramGroup)
	if err != nil {
		return err
	}
	for _, s := range list {
		if err := ignoreBusy(g.Collect(ctx, s.ID)); err != nil {
			return err
		}
	}
	return nil
}

func (g *Groups) groups(ctx context.Context, refresh bool) ([]domain.Group, error) {
	g.mu.Lock()
	cached := g.cache
	g.mu.Unlock()
	if cached != nil && !refresh {
		return cached, nil
	}
	fresh, err := g.gw.Groups(ctx)
	if err != nil {
		return nil, err
	}
	g.mu.Lock()
	g.cache = fresh
	g.mu.Unlock()
	return fresh, nil
}

func (g *Groups) find(p domain.Peer) (domain.Group, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	for _, gr := range g.cache {
		if gr.Peer == p {
			return gr, true
		}
	}
	return domain.Group{}, false
}

func (g *Groups) addedConfigs(ctx context.Context) (map[string]bool, error) {
	list, err := g.sources.ListByKind(ctx, sourcesdomain.KindTelegramGroup)
	if err != nil {
		return nil, err
	}
	added := make(map[string]bool, len(list))
	for _, s := range list {
		added[s.Config] = true
	}
	return added, nil
}

// configOf — JSON группы для sources.config (порядок полей постоянный — годится для сравнения).
func configOf(p domain.Peer) (string, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return "", fmt.Errorf("группа в JSON: %w", err)
	}
	return string(data), nil
}

func ignoreBusy(err error) error {
	if errors.Is(err, jobsdomain.ErrSourceBusy) {
		return nil
	}
	return err
}
```

- [ ] **Step 5: Запустить — должен пройти**

Run: `go test -race ./internal/telegram/service/`
Expected: PASS.

- [ ] **Step 6: Проверка**

Run: `go vet ./... && go test ./...`
Expected: без ошибок.

---

### Task 5: Сбор истории (`tg_sync`)

**Files:**
- Create: `internal/telegram/service/sync.go`, `internal/telegram/service/sync_steps.go`, `internal/telegram/service/sleep.go`
- Test: `internal/telegram/service/sync_test.go`

**Interfaces:**
- Consumes: `Gateway` (Task 4), `repository.Repository`/`NewSQLite`/`ViaAccount` (Task 3), `sourcesrepo` (Task 2), `jobsdomain.Job`, `jobsservice.Progress`, `jobsservice.WaitUserError` (M1), `texts.Tg*`
- Produces: `service.MessageStore` (`Save`, `Bounds`, `LiveIDsAfter`, `MarkDeleted`), `service.SourceGetter` (`Get(ctx, id) (sourcesdomain.Source, error)`), `service.NewSyncWorker(gw Gateway, msgs MessageStore, sources SourceGetter, now func() time.Time, sleep func(context.Context, time.Duration) error, pageSize int) *SyncWorker` — реализует `jobsservice.Worker` (`Kind() == KindSync`, `Title() == texts.TgSyncKind`); `service.SleepCtx(ctx, d) error`
- Смысл курсора (JSON): `{"phase":"new|old|deleted|done","offset":N,"top":N,"checked":N,"loaded":N,"verified":N}`. Фазы: **new** — от самого нового вниз до `top` (наибольший номер на старте); **old** — ниже наименьшего сохранённого до начала группы; **deleted** — сверка сохранённых номеров с Telegram порциями по `pageSize`.

- [ ] **Step 1: Написать падающие тесты**

`internal/telegram/service/sync_test.go`:
```go
// Файл: internal/telegram/service/sync_test.go
package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	jobsdomain "pylesos/internal/jobs/domain"
	jobsservice "pylesos/internal/jobs/service"
	sourcesdomain "pylesos/internal/sources/domain"
	sourcesrepo "pylesos/internal/sources/repository"
	"pylesos/internal/store/storetest"
	"pylesos/internal/telegram/domain"
	tgrepo "pylesos/internal/telegram/repository"
	"pylesos/internal/texts"
)

type recProgress struct {
	mu     sync.Mutex
	cursor string
	texts  []string
}

func (p *recProgress) Save(_ context.Context, cursor, text string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cursor = cursor
	p.texts = append(p.texts, text)
	return nil
}

type syncEnv struct {
	gw     *fakeGateway
	w      *SyncWorker
	msgs   *tgrepo.SQLite
	source int64
	slept  []time.Duration
}

func newSyncEnv(t *testing.T, ids ...int) *syncEnv {
	t.Helper()
	db := storetest.Open(t)
	sources := sourcesrepo.NewSQLite(db)
	cfg, err := json.Marshal(domain.Peer{Type: domain.PeerChannel, ID: 10, AccessHash: 20})
	if err != nil {
		t.Fatal(err)
	}
	id, _, err := sources.Ensure(context.Background(), sourcesdomain.Source{Kind: sourcesdomain.KindTelegramGroup,
		Title: "Тестовая группа", Config: string(cfg)})
	if err != nil {
		t.Fatal(err)
	}
	e := &syncEnv{gw: &fakeGateway{authorized: true, msgs: map[int]domain.Message{}}, msgs: tgrepo.NewSQLite(db), source: id}
	for _, n := range ids {
		e.gw.msgs[n] = domain.Message{ID: n, Date: int64(n), Text: fmt.Sprintf("сообщение %d", n)}
	}
	sleep := func(_ context.Context, d time.Duration) error { e.slept = append(e.slept, d); return nil }
	e.w = NewSyncWorker(e.gw, e.msgs, sources, func() time.Time { return time.Unix(5000, 0) }, sleep, 2)
	return e
}

func (e *syncEnv) run(t *testing.T, cursor string) (*recProgress, error) {
	t.Helper()
	p := &recProgress{}
	err := e.w.Run(context.Background(), jobsdomain.Job{SourceID: e.source, Cursor: cursor}, p)
	return p, err
}

func (e *syncEnv) count(t *testing.T) int {
	t.Helper()
	counts, err := e.msgs.Counts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return counts[e.source]
}

func TestFirstSyncLoadsWholeHistoryInPages(t *testing.T) {
	e := newSyncEnv(t, 1, 2, 3, 4, 5)
	p, err := e.run(t, "{}")
	if err != nil {
		t.Fatal(err)
	}
	if n := e.count(t); n != 5 {
		t.Fatalf("сообщений %d", n)
	}
	if !strings.Contains(p.cursor, `"phase":"done"`) || p.texts[len(p.texts)-1] != fmt.Sprintf(texts.TgProgressDone, 5) {
		t.Fatalf("курсор %s, прогресс %v", p.cursor, p.texts)
	}
}

func TestNextSyncLoadsOnlyNewAndMarksDeleted(t *testing.T) {
	e := newSyncEnv(t, 1, 2, 3, 4, 5)
	if _, err := e.run(t, "{}"); err != nil {
		t.Fatal(err)
	}
	e.gw.msgs[6] = domain.Message{ID: 6, Text: "новое"}
	e.gw.msgs[7] = domain.Message{ID: 7, Text: "ещё новое"}
	delete(e.gw.msgs, 2)
	e.gw.minIDs = nil
	if _, err := e.run(t, "{}"); err != nil {
		t.Fatal(err)
	}
	if n := e.count(t); n != 6 {
		t.Fatalf("сообщений %d (7 минус удалённое)", n)
	}
	if e.gw.minIDs[0] != 5 {
		t.Fatalf("новые запрошены не от последнего сохранённого: minID %v", e.gw.minIDs)
	}
	ids, err := e.msgs.LiveIDsAfter(context.Background(), e.source, 0, 10)
	if err != nil || len(ids) != 6 || ids[1] != 3 {
		t.Fatalf("живые %v %v", ids, err)
	}
}

func TestResumeFromCursor(t *testing.T) {
	e := newSyncEnv(t, 1, 2, 3, 4, 5)
	if _, err := e.run(t, `{"phase":"old","offset":4}`); err != nil {
		t.Fatal(err)
	}
	if n := e.count(t); n != 3 {
		t.Fatalf("после продолжения с №4 загружено %d, ждали 3 (1–3)", n)
	}
}

func TestFloodWaitWaitsAndContinues(t *testing.T) {
	e := newSyncEnv(t, 1, 2, 3, 4, 5)
	e.gw.flood = 7 * time.Second
	p, err := e.run(t, "{}")
	if err != nil {
		t.Fatal(err)
	}
	if len(e.slept) != 1 || e.slept[0] != 7*time.Second {
		t.Fatalf("ожидания %v", e.slept)
	}
	if p.texts[0] != fmt.Sprintf(texts.TgFloodWait, 7) || e.count(t) != 5 {
		t.Fatalf("прогресс %v, сообщений %d", p.texts, e.count(t))
	}
}

func TestNotAuthorizedWaitsForUser(t *testing.T) {
	e := newSyncEnv(t, 1)
	e.gw.historyErr = fmt.Errorf("сессия: %w", domain.ErrNotAuthorized)
	_, err := e.run(t, "{}")
	var wait *jobsservice.WaitUserError
	if !errors.As(err, &wait) || wait.Reason != texts.TgNeedLogin {
		t.Fatalf("ждали «войдите в Telegram», получили %v", err)
	}
}
```

- [ ] **Step 2: Запустить — должен упасть**

Run: `go test ./internal/telegram/service/ -run Sync`
Expected: FAIL — `undefined: NewSyncWorker`, `undefined: SyncWorker`.

- [ ] **Step 3: Реализовать**

`internal/telegram/service/sleep.go`:
```go
// Файл: internal/telegram/service/sleep.go
package service

import (
	"context"
	"time"
)

// SleepCtx ждёт d или отмены ctx (пауза задания, выход из программы).
func SleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
```

`internal/telegram/service/sync.go`:
```go
// Файл: internal/telegram/service/sync.go
package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	jobsdomain "pylesos/internal/jobs/domain"
	jobsservice "pylesos/internal/jobs/service"
	sourcesdomain "pylesos/internal/sources/domain"
	"pylesos/internal/telegram/domain"
	"pylesos/internal/texts"
)

type MessageStore interface {
	Save(ctx context.Context, sourceID int64, msgs []domain.Message, via string, at int64) error
	Bounds(ctx context.Context, sourceID int64) (minID, maxID int, err error)
	LiveIDsAfter(ctx context.Context, sourceID int64, afterID, limit int) ([]int, error)
	MarkDeleted(ctx context.Context, sourceID int64, ids []int) error
}

type SourceGetter interface {
	Get(ctx context.Context, id int64) (sourcesdomain.Source, error)
}

// SyncWorker — задание «сбор Telegram»: новые сообщения, история, сверка удалённых (спецификация, раздел 3).
type SyncWorker struct {
	gw       Gateway
	msgs     MessageStore
	sources  SourceGetter
	now      func() time.Time
	sleep    func(context.Context, time.Duration) error
	pageSize int
}

func NewSyncWorker(gw Gateway, msgs MessageStore, sources SourceGetter, now func() time.Time,
	sleep func(context.Context, time.Duration) error, pageSize int) *SyncWorker {
	return &SyncWorker{gw: gw, msgs: msgs, sources: sources, now: now, sleep: sleep, pageSize: pageSize}
}

func (w *SyncWorker) Kind() string  { return KindSync }
func (w *SyncWorker) Title() string { return texts.TgSyncKind }

const (
	phaseNew     = "new"
	phaseOld     = "old"
	phaseDeleted = "deleted"
	phaseDone    = "done"
)

type syncCursor struct {
	Phase    string `json:"phase"`
	Offset   int    `json:"offset"`   // загружать сообщения с номером меньше этого (0 — с самого нового)
	Top      int    `json:"top"`      // наибольший номер на старте: фаза new идёт до него
	Checked  int    `json:"checked"`  // фаза deleted: сверено до этого номера
	Loaded   int    `json:"loaded"`   // загружено за задание
	Verified int    `json:"verified"` // сверено за задание
}

type syncRun struct {
	w      *SyncWorker
	source int64
	peer   domain.Peer
	p      jobsservice.Progress
	c      syncCursor
}

func (w *SyncWorker) Run(ctx context.Context, job jobsdomain.Job, p jobsservice.Progress) error {
	run, err := w.begin(ctx, job, p)
	if err != nil {
		return err
	}
	for run.c.Phase != phaseDone {
		if err := run.step(ctx); err != nil {
			return waitIfLoggedOut(err)
		}
	}
	return nil
}

func (w *SyncWorker) begin(ctx context.Context, job jobsdomain.Job, p jobsservice.Progress) (*syncRun, error) {
	src, err := w.sources.Get(ctx, job.SourceID)
	if err != nil {
		return nil, err
	}
	var peer domain.Peer
	if err := json.Unmarshal([]byte(src.Config), &peer); err != nil {
		return nil, fmt.Errorf("группа источника %d: %w", job.SourceID, err)
	}
	var c syncCursor
	if err := json.Unmarshal([]byte(job.Cursor), &c); err != nil {
		return nil, fmt.Errorf("курсор задания: %w", err)
	}
	if c.Phase == "" {
		_, top, err := w.msgs.Bounds(ctx, job.SourceID)
		if err != nil {
			return nil, err
		}
		c = syncCursor{Phase: phaseNew, Top: top}
	}
	return &syncRun{w: w, source: job.SourceID, peer: peer, p: p, c: c}, nil
}

// waitIfLoggedOut: нет входа — задание «ждёт вас», а не «ошибка» (спецификация, раздел 7).
func waitIfLoggedOut(err error) error {
	if errors.Is(err, domain.ErrNotAuthorized) || errors.Is(err, domain.ErrNotConfigured) {
		return &jobsservice.WaitUserError{Reason: texts.TgNeedLogin}
	}
	return err
}
```

`internal/telegram/service/sync_steps.go`:
```go
// Файл: internal/telegram/service/sync_steps.go
package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"pylesos/internal/telegram/domain"
	"pylesos/internal/telegram/repository"
	"pylesos/internal/texts"
)

func (r *syncRun) step(ctx context.Context) error {
	switch r.c.Phase {
	case phaseNew:
		return r.fetch(ctx, r.c.Top, phaseOld)
	case phaseOld:
		if r.c.Offset == 0 { // в группе нет сообщений
			return r.advance(ctx, phaseDeleted)
		}
		return r.fetch(ctx, 0, phaseDeleted)
	case phaseDeleted:
		return r.checkDeleted(ctx)
	default:
		return fmt.Errorf("неизвестная фаза сбора %q", r.c.Phase)
	}
}

// fetch загружает порцию ниже Offset и выше minID; пустая порция — переход к следующей фазе.
func (r *syncRun) fetch(ctx context.Context, minID int, next string) error {
	msgs, err := retryFlood(ctx, r, func() ([]domain.Message, error) {
		return r.w.gw.History(ctx, r.peer, r.c.Offset, minID, r.w.pageSize)
	})
	if err != nil {
		return err
	}
	if len(msgs) == 0 {
		return r.advance(ctx, next)
	}
	if err := r.w.msgs.Save(ctx, r.source, msgs, repository.ViaAccount, r.w.now().Unix()); err != nil {
		return err
	}
	r.c.Offset = lowestID(msgs)
	r.c.Loaded += len(msgs)
	return r.save(ctx, r.progressText())
}

func (r *syncRun) advance(ctx context.Context, next string) error {
	switch next {
	case phaseOld:
		lo, _, err := r.w.msgs.Bounds(ctx, r.source)
		if err != nil {
			return err
		}
		r.c.Phase, r.c.Offset = phaseOld, lo
	case phaseDeleted:
		r.c.Phase, r.c.Checked = phaseDeleted, 0
	}
	return r.save(ctx, r.progressText())
}

// checkDeleted сверяет порцию сохранённых номеров с Telegram и помечает исчезнувшие.
func (r *syncRun) checkDeleted(ctx context.Context) error {
	ids, err := r.w.msgs.LiveIDsAfter(ctx, r.source, r.c.Checked, r.w.pageSize)
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		r.c.Phase = phaseDone
		return r.save(ctx, r.progressText())
	}
	alive, err := retryFlood(ctx, r, func() (map[int]bool, error) { return r.w.gw.Alive(ctx, r.peer, ids) })
	if err != nil {
		return err
	}
	gone := []int{}
	for _, id := range ids {
		if !alive[id] {
			gone = append(gone, id)
		}
	}
	if err := r.w.msgs.MarkDeleted(ctx, r.source, gone); err != nil {
		return err
	}
	r.c.Checked = ids[len(ids)-1]
	r.c.Verified += len(ids)
	return r.save(ctx, r.progressText())
}

// retryFlood повторяет вызов после FLOOD_WAIT, показав ожидание в прогрессе.
func retryFlood[T any](ctx context.Context, r *syncRun, call func() (T, error)) (T, error) {
	for {
		res, err := call()
		var flood *domain.FloodWaitError
		if !errors.As(err, &flood) {
			return res, err
		}
		var zero T
		if err := r.save(ctx, fmt.Sprintf(texts.TgFloodWait, int(flood.Wait.Seconds()))); err != nil {
			return zero, err
		}
		if err := r.w.sleep(ctx, flood.Wait); err != nil {
			return zero, err
		}
	}
}

func (r *syncRun) save(ctx context.Context, text string) error {
	data, err := json.Marshal(r.c)
	if err != nil {
		return fmt.Errorf("курсор сбора: %w", err)
	}
	return r.p.Save(ctx, string(data), text)
}

func (r *syncRun) progressText() string {
	switch r.c.Phase {
	case phaseNew:
		return fmt.Sprintf(texts.TgProgressNew, r.c.Loaded)
	case phaseOld:
		return fmt.Sprintf(texts.TgProgressOld, r.c.Loaded, r.c.Offset)
	case phaseDeleted:
		return fmt.Sprintf(texts.TgProgressDeleted, r.c.Verified)
	default:
		return fmt.Sprintf(texts.TgProgressDone, r.c.Loaded)
	}
}

func lowestID(msgs []domain.Message) int {
	lo := msgs[0].ID
	for _, m := range msgs[1:] {
		if m.ID < lo {
			lo = m.ID
		}
	}
	return lo
}
```

- [ ] **Step 4: Запустить — должен пройти**

Run: `go test -race ./internal/telegram/service/`
Expected: PASS.

- [ ] **Step 5: Проверка**

Run: `go vet ./... && go test ./...`
Expected: без ошибок.

---

### Task 6: Шлюз gotd/td

**Files:**
- Create: `internal/telegram/gotdgw/session.go`, `internal/telegram/gotdgw/errors.go`, `internal/telegram/gotdgw/mapping.go`, `internal/telegram/gotdgw/gateway.go`, `internal/telegram/gotdgw/calls.go`
- Test: `internal/telegram/gotdgw/gotdgw_test.go`

**Interfaces:**
- Consumes: `domain.*` (Task 3); `service.Gateway` — реализуется (Task 4); API из `docs/research/gotd.md`
- Produces: `gotdgw.SecretStore` (`Get`, `Put`), `gotdgw.NewSessionStorage(SecretStore) *SessionStorage` (реализует `session.Storage`), `gotdgw.KeyLoader func(ctx) (appID int, appHash string, ok bool, err error)`, `gotdgw.New(base context.Context, keys KeyLoader, storage *SessionStorage, log *slog.Logger) *Gateway` — реализует `service.Gateway`

- [ ] **Step 1: Подключить gotd**

Run: `go get github.com/gotd/td@v0.162.0`
Expected: `go.mod` получил `github.com/gotd/td v0.162.0`.

- [ ] **Step 2: Написать падающие тесты**

`internal/telegram/gotdgw/gotdgw_test.go`:
```go
// Файл: internal/telegram/gotdgw/gotdgw_test.go
package gotdgw

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"

	"pylesos/internal/telegram/domain"
)

type memSecrets map[string][]byte

func (m memSecrets) Get(_ context.Context, key string) ([]byte, bool, error) {
	v, ok := m[key]
	return v, ok, nil
}

func (m memSecrets) Put(_ context.Context, key string, value []byte) error {
	m[key] = value
	return nil
}

func TestSessionStorage(t *testing.T) {
	ctx := context.Background()
	s := NewSessionStorage(memSecrets{})
	if _, err := s.LoadSession(ctx); !errors.Is(err, session.ErrNotFound) {
		t.Fatalf("нет сессии: %v", err)
	}
	if err := s.StoreSession(ctx, []byte("сессия")); err != nil {
		t.Fatal(err)
	}
	if got, err := s.LoadSession(ctx); err != nil || string(got) != "сессия" {
		t.Fatalf("got %q %v", got, err)
	}
}

func TestMapErr(t *testing.T) {
	cases := []struct {
		in   error
		want error
	}{
		{tgerr.New(400, "PHONE_CODE_INVALID"), domain.ErrBadCode},
		{tgerr.New(400, "PHONE_CODE_EXPIRED"), domain.ErrCodeExpired},
		{tgerr.New(400, "PHONE_NUMBER_INVALID"), domain.ErrBadPhone},
		{tgerr.New(400, "PASSWORD_HASH_INVALID"), domain.ErrBadPassword},
		{tgerr.New(401, "AUTH_KEY_UNREGISTERED"), domain.ErrNotAuthorized},
		{auth.ErrPasswordAuthNeeded, domain.ErrPasswordNeeded},
	}
	for _, c := range cases {
		if got := mapErr(c.in); !errors.Is(got, c.want) {
			t.Fatalf("%v → %v, ждали %v", c.in, got, c.want)
		}
	}
	var flood *domain.FloodWaitError
	if !errors.As(mapErr(tgerr.New(420, "FLOOD_WAIT_7")), &flood) || flood.Wait != 7*time.Second {
		t.Fatalf("FLOOD_WAIT: %+v", flood)
	}
	if mapErr(nil) != nil {
		t.Fatal("nil должен остаться nil")
	}
}

func TestMessagesFrom(t *testing.T) {
	m := &tg.Message{ID: 42, Date: 1700000000, Message: "Текст о водителе"}
	m.SetFromID(&tg.PeerUser{UserID: 5})
	reply := &tg.MessageReplyHeader{}
	reply.SetReplyToMsgID(40)
	m.SetReplyTo(reply)
	m.SetEditDate(1700000100)
	fwd := tg.MessageFwdHeader{}
	fwd.SetFromName("Другой автопарк")
	m.SetFwdFrom(fwd)
	photo := &tg.MessageMediaPhoto{}
	photo.SetPhoto(&tg.Photo{ID: 99})
	m.SetMedia(photo)
	user := &tg.User{ID: 5}
	user.SetFirstName("Иван")
	user.SetLastName("Тестов")
	res := &tg.MessagesMessages{
		Messages: []tg.MessageClass{m, &tg.MessageService{ID: 43}},
		Users:    []tg.UserClass{user},
	}
	msgs, err := messagesFrom(res)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 {
		t.Fatalf("служебные сообщения не пропущены: %d", len(msgs))
	}
	got := msgs[0]
	if got.ID != 42 || got.AuthorID != 5 || got.AuthorName != "Иван Тестов" || got.ReplyTo != 40 ||
		got.EditDate != 1700000100 || got.FwdFrom != "Другой автопарк" || got.Text != "Текст о водителе" {
		t.Fatalf("разбор: %+v", got)
	}
	if len(got.Media) != 1 || got.Media[0].Kind != domain.MediaPhoto || got.Media[0].FileID != 99 || got.Raw == "" {
		t.Fatalf("вложение/raw: %+v", got)
	}
}

func TestDocumentAndAlive(t *testing.T) {
	doc := &tg.Document{ID: 7, MimeType: "application/pdf", Size: 2048,
		Attributes: []tg.DocumentAttributeClass{&tg.DocumentAttributeFilename{FileName: "удостоверение.pdf"}}}
	m := &tg.Message{ID: 1}
	media := &tg.MessageMediaDocument{}
	media.SetDocument(doc)
	m.SetMedia(media)
	msgs, err := messagesFrom(&tg.MessagesMessages{Messages: []tg.MessageClass{m}})
	if err != nil || msgs[0].Media[0].FileName != "удостоверение.pdf" || msgs[0].Media[0].Kind != domain.MediaDocument {
		t.Fatalf("документ: %+v %v", msgs, err)
	}
	alive, err := aliveFrom(&tg.MessagesChannelMessages{Messages: []tg.MessageClass{&tg.Message{ID: 1}, &tg.MessageEmpty{ID: 2}}})
	if err != nil || !alive[1] || alive[2] {
		t.Fatalf("живые: %v %v", alive, err)
	}
}

func TestInputPeer(t *testing.T) {
	ch, ok := inputPeer(domain.Peer{Type: domain.PeerChannel, ID: 1, AccessHash: 2}).(*tg.InputPeerChannel)
	if !ok || ch.ChannelID != 1 || ch.AccessHash != 2 {
		t.Fatalf("канал: %+v", ch)
	}
	if _, ok := inputPeer(domain.Peer{Type: domain.PeerChat, ID: 3}).(*tg.InputPeerChat); !ok {
		t.Fatal("обычная группа")
	}
}
```

- [ ] **Step 3: Запустить — должен упасть**

Run: `go test ./internal/telegram/gotdgw/`
Expected: FAIL — `undefined: NewSessionStorage`, `undefined: mapErr` и др.

- [ ] **Step 4: Реализовать**

`internal/telegram/gotdgw/session.go`:
```go
// Файл: internal/telegram/gotdgw/session.go
// Package gotdgw — шлюз к Telegram через gotd/td (MTProto): вход, группы, история (П6, П14).
package gotdgw

import (
	"context"

	"github.com/gotd/td/session"
)

const secretSession = "telegram_session"

// SecretStore — шифрованные секреты (internal/secrets).
type SecretStore interface {
	Get(ctx context.Context, key string) ([]byte, bool, error)
	Put(ctx context.Context, key string, value []byte) error
}

// SessionStorage хранит сессию Telegram в зашифрованных секретах (10-security.md).
type SessionStorage struct{ secrets SecretStore }

func NewSessionStorage(s SecretStore) *SessionStorage { return &SessionStorage{secrets: s} }

func (s *SessionStorage) LoadSession(ctx context.Context) ([]byte, error) {
	data, ok, err := s.secrets.Get(ctx, secretSession)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, session.ErrNotFound
	}
	return data, nil
}

func (s *SessionStorage) StoreSession(ctx context.Context, data []byte) error {
	return s.secrets.Put(ctx, secretSession, data)
}
```

`internal/telegram/gotdgw/errors.go`:
```go
// Файл: internal/telegram/gotdgw/errors.go
package gotdgw

import (
	"errors"
	"fmt"

	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/tgerr"

	"pylesos/internal/telegram/domain"
)

// mapErr переводит ошибки Telegram в ошибки domain; остальные — как есть.
func mapErr(err error) error {
	if err == nil {
		return nil
	}
	if d, ok := tgerr.AsFloodWait(err); ok {
		return &domain.FloodWaitError{Wait: d}
	}
	switch {
	case errors.Is(err, auth.ErrPasswordAuthNeeded):
		return domain.ErrPasswordNeeded
	case tgerr.Is(err, "PHONE_NUMBER_INVALID", "PHONE_NUMBER_BANNED", "PHONE_NUMBER_FLOOD"):
		return fmt.Errorf("%w: %w", domain.ErrBadPhone, err)
	case tgerr.Is(err, "PHONE_CODE_INVALID", "PHONE_CODE_EMPTY"):
		return fmt.Errorf("%w: %w", domain.ErrBadCode, err)
	case tgerr.Is(err, "PHONE_CODE_EXPIRED"):
		return fmt.Errorf("%w: %w", domain.ErrCodeExpired, err)
	case tgerr.Is(err, "PASSWORD_HASH_INVALID"):
		return fmt.Errorf("%w: %w", domain.ErrBadPassword, err)
	case tgerr.Is(err, "AUTH_KEY_UNREGISTERED", "SESSION_REVOKED", "SESSION_EXPIRED", "USER_DEACTIVATED"):
		return fmt.Errorf("%w: %w", domain.ErrNotAuthorized, err)
	}
	return err
}
```

`internal/telegram/gotdgw/mapping.go`:
```go
// Файл: internal/telegram/gotdgw/mapping.go
package gotdgw

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gotd/td/telegram/query/dialogs"
	"github.com/gotd/td/tg"

	"pylesos/internal/telegram/domain"
)

// messagesFrom разбирает ответ истории: только обычные сообщения, служебные («вступил в группу») пропускаются.
func messagesFrom(res tg.MessagesMessagesClass) ([]domain.Message, error) {
	mod, ok := res.AsModified()
	if !ok {
		return nil, fmt.Errorf("неожиданный ответ истории %T", res)
	}
	users := map[int64]*tg.User{}
	for _, u := range mod.GetUsers() {
		if user, ok := u.(*tg.User); ok {
			users[user.GetID()] = user
		}
	}
	chats := map[int64]string{}
	for _, c := range mod.GetChats() {
		switch ch := c.(type) {
		case *tg.Channel:
			chats[ch.GetID()] = ch.GetTitle()
		case *tg.Chat:
			chats[ch.GetID()] = ch.GetTitle()
		}
	}
	out := []domain.Message{}
	for _, m := range mod.GetMessages() {
		if msg, ok := m.(*tg.Message); ok {
			dm, err := toMessage(msg, users, chats)
			if err != nil {
				return nil, err
			}
			out = append(out, dm)
		}
	}
	return out, nil
}

func toMessage(m *tg.Message, users map[int64]*tg.User, chats map[int64]string) (domain.Message, error) {
	raw, err := json.Marshal(m)
	if err != nil {
		return domain.Message{}, fmt.Errorf("сообщение %d в JSON: %w", m.GetID(), err)
	}
	out := domain.Message{ID: m.GetID(), Date: int64(m.GetDate()), Text: m.GetMessage(), Media: mediaOf(m), Raw: string(raw)}
	if from, ok := m.GetFromID(); ok {
		out.AuthorID, out.AuthorName = author(from, users, chats)
	}
	if edit, ok := m.GetEditDate(); ok {
		out.EditDate = int64(edit)
	}
	if reply, ok := m.GetReplyTo(); ok {
		if h, ok := reply.(*tg.MessageReplyHeader); ok {
			out.ReplyTo, _ = h.GetReplyToMsgID()
			out.ThreadTop, _ = h.GetReplyToTopID()
		}
	}
	if fwd, ok := m.GetFwdFrom(); ok {
		out.FwdFrom = forwardedFrom(fwd, users, chats)
	}
	return out, nil
}

func author(p tg.PeerClass, users map[int64]*tg.User, chats map[int64]string) (int64, string) {
	switch v := p.(type) {
	case *tg.PeerUser:
		return v.GetUserID(), userName(users[v.GetUserID()])
	case *tg.PeerChannel:
		return v.GetChannelID(), chats[v.GetChannelID()]
	case *tg.PeerChat:
		return v.GetChatID(), chats[v.GetChatID()]
	}
	return 0, ""
}

func userName(u *tg.User) string {
	if u == nil {
		return ""
	}
	first, _ := u.GetFirstName()
	last, _ := u.GetLastName()
	if name := strings.TrimSpace(first + " " + last); name != "" {
		return name
	}
	if nick, ok := u.GetUsername(); ok {
		return "@" + nick
	}
	return ""
}

func forwardedFrom(f tg.MessageFwdHeader, users map[int64]*tg.User, chats map[int64]string) string {
	if name, ok := f.GetFromName(); ok {
		return name
	}
	if from, ok := f.GetFromID(); ok {
		_, name := author(from, users, chats)
		return name
	}
	return ""
}

func mediaOf(m *tg.Message) []domain.Media {
	media, ok := m.GetMedia()
	if !ok {
		return nil
	}
	switch v := media.(type) {
	case *tg.MessageMediaPhoto:
		if p, ok := v.GetPhoto(); ok {
			if photo, ok := p.(*tg.Photo); ok {
				return []domain.Media{{Kind: domain.MediaPhoto, FileID: photo.GetID(), MimeType: "image/jpeg"}}
			}
		}
	case *tg.MessageMediaDocument:
		if d, ok := v.GetDocument(); ok {
			if doc, ok := d.(*tg.Document); ok {
				return []domain.Media{documentOf(doc)}
			}
		}
	}
	return nil
}

func documentOf(d *tg.Document) domain.Media {
	md := domain.Media{Kind: domain.MediaDocument, FileID: d.GetID(), MimeType: d.GetMimeType(), Size: d.GetSize()}
	for _, a := range d.GetAttributes() {
		if fn, ok := a.(*tg.DocumentAttributeFilename); ok {
			md.FileName = fn.GetFileName()
		}
	}
	return md
}

// groupOf — группа из элемента списка диалогов; каналы-рассылки и личные чаты пропускаются.
func groupOf(e dialogs.Elem) (domain.Group, bool) {
	switch p := e.Peer.(type) {
	case *tg.InputPeerChannel:
		ch, ok := e.Entities.Channel(p.ChannelID)
		if !ok || !ch.GetMegagroup() {
			return domain.Group{}, false
		}
		return domain.Group{Peer: domain.Peer{Type: domain.PeerChannel, ID: p.ChannelID, AccessHash: p.AccessHash}, Title: ch.GetTitle()}, true
	case *tg.InputPeerChat:
		c, ok := e.Entities.Chat(p.ChatID)
		if !ok {
			return domain.Group{}, false
		}
		return domain.Group{Peer: domain.Peer{Type: domain.PeerChat, ID: p.ChatID}, Title: c.GetTitle()}, true
	}
	return domain.Group{}, false
}

func inputPeer(p domain.Peer) tg.InputPeerClass {
	if p.Type == domain.PeerChat {
		return &tg.InputPeerChat{ChatID: p.ID}
	}
	return &tg.InputPeerChannel{ChannelID: p.ID, AccessHash: p.AccessHash}
}

// aliveFrom — номера сообщений, которые ещё существуют (удалённое приходит как MessageEmpty).
func aliveFrom(res tg.MessagesMessagesClass) (map[int]bool, error) {
	mod, ok := res.AsModified()
	if !ok {
		return nil, fmt.Errorf("неожиданный ответ %T", res)
	}
	alive := map[int]bool{}
	for _, m := range mod.GetMessages() {
		if _, empty := m.(*tg.MessageEmpty); !empty {
			alive[m.GetID()] = true
		}
	}
	return alive, nil
}
```

`internal/telegram/gotdgw/gateway.go`:
```go
// Файл: internal/telegram/gotdgw/gateway.go
package gotdgw

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/gotd/td/telegram"

	"pylesos/internal/telegram/domain"
)

// KeyLoader — api_id и api_hash (вводит пользователь, П19); ok=false — ещё не введены.
type KeyLoader func(ctx context.Context) (appID int, appHash string, ok bool, err error)

// Gateway — одно соединение с Telegram на программу; поднимается при первом обращении.
type Gateway struct {
	base    context.Context // отменяется при выходе из программы
	keys    KeyLoader
	storage *SessionStorage
	log     *slog.Logger

	mu     sync.Mutex
	client *telegram.Client
	stop   context.CancelFunc
}

func New(base context.Context, keys KeyLoader, storage *SessionStorage, log *slog.Logger) *Gateway {
	return &Gateway{base: base, keys: keys, storage: storage, log: log}
}

// Reset разрывает соединение: следующий вызов подключится с новыми ключами.
func (g *Gateway) Reset() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.stop != nil {
		g.stop()
	}
	g.client, g.stop = nil, nil
}

func (g *Gateway) conn(ctx context.Context) (*telegram.Client, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.client != nil {
		return g.client, nil
	}
	appID, appHash, ok, err := g.keys(ctx)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, domain.ErrNotConfigured
	}
	client := telegram.NewClient(appID, appHash, telegram.Options{SessionStorage: g.storage, NoUpdates: true})
	runCtx, stop := context.WithCancel(g.base)
	ready := make(chan error, 1)
	go g.run(runCtx, client, ready)
	select {
	case err := <-ready:
		if err != nil {
			stop()
			return nil, fmt.Errorf("подключиться к Telegram: %w", err)
		}
	case <-ctx.Done():
		stop()
		return nil, ctx.Err()
	}
	g.client, g.stop = client, stop
	return client, nil
}

// run держит соединение, пока не отменён ctx; в ready — подключились ли.
func (g *Gateway) run(ctx context.Context, client *telegram.Client, ready chan<- error) {
	err := client.Run(ctx, func(ctx context.Context) error {
		ready <- nil
		<-ctx.Done()
		return ctx.Err()
	})
	select {
	case ready <- err:
	default:
	}
	g.mu.Lock()
	if g.client == client {
		g.client, g.stop = nil, nil
	}
	g.mu.Unlock()
	if err != nil && !errors.Is(err, context.Canceled) {
		g.log.Warn("соединение с Telegram закрыто", "err", err)
	}
}
```

`internal/telegram/gotdgw/calls.go`:
```go
// Файл: internal/telegram/gotdgw/calls.go
package gotdgw

import (
	"context"
	"errors"
	"fmt"

	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/telegram/query"
	"github.com/gotd/td/tg"

	"pylesos/internal/telegram/domain"
)

func (g *Gateway) Account(ctx context.Context) (domain.Account, error) {
	c, err := g.conn(ctx)
	if err != nil {
		return domain.Account{}, err
	}
	st, err := c.Auth().Status(ctx)
	if err := mapErr(err); errors.Is(err, domain.ErrNotAuthorized) {
		return domain.Account{}, nil
	} else if err != nil {
		return domain.Account{}, err
	}
	if !st.Authorized {
		return domain.Account{}, nil
	}
	return domain.Account{Authorized: true, Name: userName(st.User)}, nil
}

func (g *Gateway) SendCode(ctx context.Context, phone string) (string, error) {
	c, err := g.conn(ctx)
	if err != nil {
		return "", err
	}
	sent, err := c.Auth().SendCode(ctx, phone, auth.SendCodeOptions{})
	if err != nil {
		return "", mapErr(err)
	}
	code, ok := sent.(*tg.AuthSentCode)
	if !ok {
		return "", fmt.Errorf("неожиданный ответ на запрос кода %T", sent)
	}
	return code.GetPhoneCodeHash(), nil
}

func (g *Gateway) SignIn(ctx context.Context, phone, code, codeHash string) error {
	c, err := g.conn(ctx)
	if err != nil {
		return err
	}
	_, err = c.Auth().SignIn(ctx, phone, code, codeHash)
	return mapErr(err)
}

func (g *Gateway) Password(ctx context.Context, password string) error {
	c, err := g.conn(ctx)
	if err != nil {
		return err
	}
	_, err = c.Auth().Password(ctx, password)
	return mapErr(err)
}

func (g *Gateway) Groups(ctx context.Context) ([]domain.Group, error) {
	c, err := g.conn(ctx)
	if err != nil {
		return nil, err
	}
	iter := query.GetDialogs(c.API()).BatchSize(100).Iter()
	groups := []domain.Group{}
	for iter.Next(ctx) {
		if gr, ok := groupOf(iter.Value()); ok {
			groups = append(groups, gr)
		}
	}
	if err := iter.Err(); err != nil {
		return nil, mapErr(err)
	}
	return groups, nil
}

func (g *Gateway) History(ctx context.Context, p domain.Peer, offsetID, minID, limit int) ([]domain.Message, error) {
	c, err := g.conn(ctx)
	if err != nil {
		return nil, err
	}
	res, err := c.API().MessagesGetHistory(ctx, &tg.MessagesGetHistoryRequest{
		Peer: inputPeer(p), OffsetID: offsetID, MinID: minID, Limit: limit})
	if err != nil {
		return nil, mapErr(err)
	}
	return messagesFrom(res)
}

func (g *Gateway) Alive(ctx context.Context, p domain.Peer, ids []int) (map[int]bool, error) {
	c, err := g.conn(ctx)
	if err != nil {
		return nil, err
	}
	inputs := make([]tg.InputMessageClass, 0, len(ids))
	for _, id := range ids {
		inputs = append(inputs, &tg.InputMessageID{ID: id})
	}
	var res tg.MessagesMessagesClass
	if p.Type == domain.PeerChat {
		res, err = c.API().MessagesGetMessages(ctx, inputs)
	} else {
		res, err = c.API().ChannelsGetMessages(ctx, &tg.ChannelsGetMessagesRequest{
			Channel: &tg.InputChannel{ChannelID: p.ID, AccessHash: p.AccessHash}, ID: inputs})
	}
	if err != nil {
		return nil, mapErr(err)
	}
	return aliveFrom(res)
}
```

- [ ] **Step 5: Запустить — должен пройти**

Run: `go test -race ./internal/telegram/gotdgw/ && GOOS=windows go vet ./internal/telegram/... && GOOS=darwin go vet ./internal/telegram/...`
Expected: PASS; сборка под Windows и macOS без ошибок.

- [ ] **Step 6: Проверка**

Run: `go vet ./... && go test ./...`
Expected: без ошибок.

---

### Task 7: Экраны «Telegram» и группы на «Источниках»

**Files:**
- Modify: `internal/jobs/domain/job.go` (`SourceStats`), `internal/jobs/repository/repository.go`, `internal/jobs/repository/sqlite.go`, Test: `internal/jobs/repository/sqlite_test.go`
- Modify: `internal/screens/types.go`, `internal/screens/builder.go`, `internal/screens/jobs.go`
- Create: `internal/screens/home.go`, `internal/screens/telegram.go`
- Modify/Test: `internal/screens/screens_test.go`, Create test: `internal/screens/telegram_test.go`

**Interfaces:**
- Consumes: `tgservice.LoginState/Step*/Field*/GroupView/ErrGroupGone` (Task 4), `sourcesdomain` (Task 2), `tgdomain.Peer.Key` (Task 3)
- Produces:
  - `jobsdomain.SourceStats{SourceID int64; Active bool; LastDone int64}`; `jobsrepository.Repository.SourceStats(ctx) (map[int64]jobsdomain.SourceStats, error)`
  - `screens.Deps{Jobs JobLister; Kinds KindTitler; Settings SettingsStore; Sources SourceLister; Counts MessageCounter; Stats JobStats; Login TelegramLogin; Groups TelegramGroups; Loc *time.Location}`; `screens.NewBuilder(d Deps) *Builder` (**меняется сигнатура M1**)
  - Порты: `SourceLister{ListByKind}`, `MessageCounter{Counts}`, `JobStats{SourceStats}`, `TelegramLogin{State; Submit}`, `TelegramGroups{List(ctx, refresh bool) ([]tgservice.GroupView, error); Add(ctx, key string) error}`
  - `HomeScreen` + поля `telegram_title`, `telegram []SourceRow`; `SourceRow{ID, Title, Lines []string, Actions []Action}`; `TelegramScreen{Title, Hint, Fields, SubmitLabel, SavingLabel, Status, Message, GroupsTitle, GroupsEmpty, RefreshLabel, Groups []GroupRow}`; `GroupRow{Key, Title, Status, Action *Action}`; пункт меню `telegram`
  - Методы `Builder`: `Telegram(ctx)`, `TelegramSubmit(ctx, values)`, `TelegramAddGroup(ctx, key)`, `TelegramRefresh(ctx)` → `(TelegramScreen, error)`

- [ ] **Step 1: Статистика заданий по источникам — тест и реализация**

Дописать в `internal/jobs/repository/sqlite_test.go`:
```go
func TestSourceStats(t *testing.T) {
	ctx := context.Background()
	db := storetest.Open(t)
	a := storetest.AddSource(t, db, "А")
	b := storetest.AddSource(t, db, "Б")
	repo := NewSQLite(db)
	done, err := repo.Create(ctx, newJob(a, domain.StatusQueued))
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.UpdateState(ctx, done, domain.StateUpdate{Status: domain.StatusDone, At: 700, Finished: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Create(ctx, newJob(b, domain.StatusRunning)); err != nil {
		t.Fatal(err)
	}
	stats, err := repo.SourceStats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats[a].Active || stats[a].LastDone != 700 || !stats[b].Active || stats[b].LastDone != 0 {
		t.Fatalf("статистика: %+v", stats)
	}
}
```

Run: `go test ./internal/jobs/repository/` → Expected: FAIL (`repo.SourceStats undefined`).

Дописать в `internal/jobs/domain/job.go`:
```go
// SourceStats — сводка заданий источника для экранов.
type SourceStats struct {
	SourceID int64 `db:"source_id"`
	Active   bool  `db:"active"`    // есть активное задание
	LastDone int64 `db:"last_done"` // когда последнее задание завершилось успешно (0 — никогда)
}
```

В интерфейс `internal/jobs/repository/repository.go` добавить:
```go
	SourceStats(ctx context.Context) (map[int64]domain.SourceStats, error)
```

Дописать в `internal/jobs/repository/sqlite.go`:
```go
func (r *SQLite) SourceStats(ctx context.Context) (map[int64]domain.SourceStats, error) {
	var rows []domain.SourceStats
	if err := r.db.SelectContext(ctx, &rows, `SELECT source_id,
		MAX(status IN ('queued', 'running', 'paused', 'waiting_user')) AS active,
		COALESCE(MAX(CASE WHEN status = 'done' THEN finished_at END), 0) AS last_done
		FROM jobs GROUP BY source_id`); err != nil {
		return nil, fmt.Errorf("сводка заданий по источникам: %w", err)
	}
	out := make(map[int64]domain.SourceStats, len(rows))
	for _, s := range rows {
		out[s.SourceID] = s
	}
	return out, nil
}
```

Run: `go test ./internal/jobs/...` → Expected: PASS.

- [ ] **Step 2: Написать падающие тесты экранов**

В `internal/screens/screens_test.go` заменить функцию `builder` и добавить подставки:
```go
type fakeSources []sourcesdomain.Source

func (f fakeSources) ListByKind(context.Context, sourcesdomain.Kind) ([]sourcesdomain.Source, error) {
	return f, nil
}

type fakeCounts map[int64]int

func (f fakeCounts) Counts(context.Context) (map[int64]int, error) { return f, nil }

type fakeStats map[int64]jobsdomain.SourceStats

func (f fakeStats) SourceStats(context.Context) (map[int64]jobsdomain.SourceStats, error) { return f, nil }

func builder(jobs fakeJobs, s *fakeSettings) *Builder {
	return NewBuilder(Deps{Jobs: jobs, Kinds: fakeKinds{}, Settings: s, Sources: fakeSources{},
		Counts: fakeCounts{}, Stats: fakeStats{}, Login: &fakeLogin{}, Groups: &fakeGroups{}, Loc: time.UTC})
}
```
(импорт `sourcesdomain "pylesos/internal/sources/domain"`).

`internal/screens/telegram_test.go`:
```go
// Файл: internal/screens/telegram_test.go
package screens

import (
	"context"
	"fmt"
	"testing"
	"time"

	tgdomain "pylesos/internal/telegram/domain"
	tgservice "pylesos/internal/telegram/service"
	"pylesos/internal/texts"
)

type fakeLogin struct {
	state    tgservice.LoginState
	problems map[string]string
	got      map[string]string
}

func (f *fakeLogin) State(context.Context) (tgservice.LoginState, error) { return f.state, nil }
func (f *fakeLogin) Submit(_ context.Context, v map[string]string) (map[string]string, error) {
	f.got = v
	return f.problems, nil
}

type fakeGroups struct {
	views   []tgservice.GroupView
	addErr  error
	added   string
	refresh bool
}

func (f *fakeGroups) List(_ context.Context, refresh bool) ([]tgservice.GroupView, error) {
	f.refresh = refresh
	return f.views, nil
}
func (f *fakeGroups) Add(_ context.Context, key string) error { f.added = key; return f.addErr }

func tgBuilder(l *fakeLogin, g *fakeGroups) *Builder {
	return NewBuilder(Deps{Jobs: fakeJobs{}, Kinds: fakeKinds{}, Settings: &fakeSettings{}, Sources: fakeSources{},
		Counts: fakeCounts{}, Stats: fakeStats{}, Login: l, Groups: g, Loc: time.UTC})
}

func TestTelegramLoginStepKeepsInputExceptPassword(t *testing.T) {
	l := &fakeLogin{state: tgservice.LoginState{Step: tgservice.StepKeys},
		problems: map[string]string{tgservice.FieldAPIHash: texts.ErrTgAPIHash}}
	s, err := tgBuilder(l, &fakeGroups{}).TelegramSubmit(context.Background(),
		map[string]string{tgservice.FieldAPIID: "123", tgservice.FieldAPIHash: "плохой"})
	if err != nil {
		t.Fatal(err)
	}
	if s.Hint != texts.TgKeysHint || len(s.Fields) != 2 || s.Fields[0].Value != "123" || s.Fields[1].Error != texts.ErrTgAPIHash {
		t.Fatalf("шаг ключей: %+v", s)
	}
	l.state = tgservice.LoginState{Step: tgservice.StepPassword}
	l.problems = map[string]string{tgservice.FieldPassword: texts.ErrTgBadPassword}
	s, err = tgBuilder(l, &fakeGroups{}).TelegramSubmit(context.Background(), map[string]string{tgservice.FieldPassword: "секрет"})
	if err != nil {
		t.Fatal(err)
	}
	if s.Fields[0].Kind != "password" || s.Fields[0].Value != "" || s.Fields[0].Error != texts.ErrTgBadPassword {
		t.Fatalf("пароль вернулся в форму или нет ошибки: %+v", s.Fields[0])
	}
	l.state = tgservice.LoginState{Step: tgservice.StepCode, Phone: "+77011234567"}
	s, err = tgBuilder(l, &fakeGroups{}).Telegram(context.Background())
	if err != nil || s.Hint != fmt.Sprintf(texts.TgCodeHint, "+77011234567") {
		t.Fatalf("подсказка кода: %q %v", s.Hint, err)
	}
}

func TestTelegramReadyListsGroups(t *testing.T) {
	g := &fakeGroups{views: []tgservice.GroupView{
		{Group: tgdomain.Group{Peer: tgdomain.Peer{Type: tgdomain.PeerChannel, ID: 1, AccessHash: 2}, Title: "Новая"}},
		{Group: tgdomain.Group{Peer: tgdomain.Peer{Type: tgdomain.PeerChat, ID: 3}, Title: "Добавленная"}, Added: true},
	}}
	b := tgBuilder(&fakeLogin{state: tgservice.LoginState{Step: tgservice.StepReady, Name: "Тест"}}, g)
	s, err := b.TelegramRefresh(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !g.refresh || s.Status != fmt.Sprintf(texts.TgLoggedIn, "Тест") || len(s.Fields) != 0 {
		t.Fatalf("экран: %+v", s)
	}
	if s.Groups[0].Key != "channel:1:2" || s.Groups[0].Action == nil || s.Groups[1].Action != nil || s.Groups[1].Status != texts.TgAdded {
		t.Fatalf("группы: %+v", s.Groups)
	}
	g.addErr = tgservice.ErrGroupGone
	if s, err := b.TelegramAddGroup(context.Background(), "channel:1:2"); err != nil || s.Message != texts.ErrTgGroupGone || g.added != "channel:1:2" {
		t.Fatalf("исчезнувшая группа: %+v %v", s, err)
	}
	g.addErr = nil
	if s, err := b.TelegramAddGroup(context.Background(), "channel:1:2"); err != nil || s.Message != texts.TgGroupAdded {
		t.Fatalf("добавление: %+v %v", s, err)
	}
}

func TestHomeTelegramRows(t *testing.T) {
	b := NewBuilder(Deps{Jobs: fakeJobs{}, Kinds: fakeKinds{}, Settings: &fakeSettings{},
		Sources: fakeSources{{ID: 1, Title: "Группа А"}, {ID: 2, Title: "Группа Б"}},
		Counts:  fakeCounts{1: 124},
		Stats:   fakeStats{1: {SourceID: 1, LastDone: time.Date(2026, 9, 27, 9, 30, 0, 0, time.UTC).Unix()}, 2: {SourceID: 2, Active: true}},
		Login:   &fakeLogin{}, Groups: &fakeGroups{}, Loc: time.UTC})
	s, err := b.Home(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if s.EmptyText != "" || s.TelegramTitle != texts.HomeTelegramTitle || len(s.Telegram) != 2 {
		t.Fatalf("главная: %+v", s)
	}
	a := s.Telegram[0]
	if a.Lines[0] != fmt.Sprintf(texts.TgMessagesCount, 124) || a.Lines[1] != fmt.Sprintf(texts.TgLastSync, "27.09.2026 09:30") ||
		len(a.Actions) != 1 || a.Actions[0].ID != "collect" {
		t.Fatalf("группа А: %+v", a)
	}
	if bb := s.Telegram[1]; bb.Lines[1] != texts.TgNeverSynced || len(bb.Actions) != 0 {
		t.Fatalf("группа Б (идёт сбор): %+v", bb)
	}
}
```

Run: `go test ./internal/screens/`
Expected: FAIL — `undefined: Deps` и др.

- [ ] **Step 3: Реализовать**

В `internal/screens/types.go` добавить в `HomeScreen` поля и новые типы:
```go
type HomeScreen struct {
	Title         string      `json:"title"`
	Banners       []Banner    `json:"banners"`
	EmptyText     string      `json:"empty_text"` // пусто, когда источники есть
	TelegramTitle string      `json:"telegram_title"`
	Telegram      []SourceRow `json:"telegram"`
}

// SourceRow — источник на главной: строки сведений и кнопки.
type SourceRow struct {
	ID      int64    `json:"id"`
	Title   string   `json:"title"`
	Lines   []string `json:"lines"`
	Actions []Action `json:"actions"`
}

// TelegramScreen — вход по шагам; после входа — группы аккаунта.
type TelegramScreen struct {
	Title        string     `json:"title"`
	Hint         string     `json:"hint"`
	Fields       []Field    `json:"fields"` // поля текущего шага; пусто, когда вход выполнен
	SubmitLabel  string     `json:"submit_label"`
	SavingLabel  string     `json:"saving_label"`
	Status       string     `json:"status"`
	Message      string     `json:"message"`
	GroupsTitle  string     `json:"groups_title"`
	GroupsEmpty  string     `json:"groups_empty"`
	RefreshLabel string     `json:"refresh_label"`
	Groups       []GroupRow `json:"groups"`
}

// GroupRow — группа аккаунта; Action nil — уже добавлена (тогда Status).
type GroupRow struct {
	Key    string  `json:"key"`
	Title  string  `json:"title"`
	Status string  `json:"status"`
	Action *Action `json:"action"`
}
```
и в комментарий `Field.Kind`: `"text" | "number" | "password"`.

`internal/screens/builder.go` (целиком заменить):
```go
// Файл: internal/screens/builder.go
package screens

import (
	"context"
	"time"

	jobsdomain "pylesos/internal/jobs/domain"
	settingsdomain "pylesos/internal/settings/domain"
	sourcesdomain "pylesos/internal/sources/domain"
	tgservice "pylesos/internal/telegram/service"
	"pylesos/internal/texts"
)

type JobLister interface {
	List(ctx context.Context) ([]jobsdomain.ListItem, error)
}

type KindTitler interface {
	KindTitle(kind string) string
}

type SettingsStore interface {
	Get(ctx context.Context) (settingsdomain.Settings, error)
	Save(ctx context.Context, input map[string]string) (settingsdomain.Settings, map[string]string, error)
}

type SourceLister interface {
	ListByKind(ctx context.Context, kind sourcesdomain.Kind) ([]sourcesdomain.Source, error)
}

type MessageCounter interface {
	Counts(ctx context.Context) (map[int64]int, error)
}

type JobStats interface {
	SourceStats(ctx context.Context) (map[int64]jobsdomain.SourceStats, error)
}

type TelegramLogin interface {
	State(ctx context.Context) (tgservice.LoginState, error)
	Submit(ctx context.Context, values map[string]string) (map[string]string, error)
}

type TelegramGroups interface {
	List(ctx context.Context, refresh bool) ([]tgservice.GroupView, error)
	Add(ctx context.Context, key string) error
}

// Deps — откуда экраны берут данные.
type Deps struct {
	Jobs     JobLister
	Kinds    KindTitler
	Settings SettingsStore
	Sources  SourceLister
	Counts   MessageCounter
	Stats    JobStats
	Login    TelegramLogin
	Groups   TelegramGroups
	Loc      *time.Location
}

type Builder struct{ d Deps }

func NewBuilder(d Deps) *Builder { return &Builder{d: d} }

func (b *Builder) Layout() Layout {
	return Layout{
		AppName: texts.AppName,
		Nav: []NavItem{
			{ID: "home", Label: texts.NavHome},
			{ID: "telegram", Label: texts.NavTelegram},
			{ID: "jobs", Label: texts.NavJobs},
			{ID: "settings", Label: texts.NavSettings},
		},
		ThemeLabel: texts.ThemeToggle,
		QuitLabel:  texts.Quit,
		ClosedText: texts.ProgramClosed,
	}
}

// formatTime — время для экранов: «27.09.2026 09:30» в часовом поясе пользователя.
func (b *Builder) formatTime(unix int64) string {
	return time.Unix(unix, 0).In(b.d.Loc).Format("02.01.2006 15:04")
}
```

В `internal/screens/jobs.go` и `internal/screens/settings.go` заменить обращения `b.jobs` → `b.d.Jobs`, `b.kinds` → `b.d.Kinds`, `b.settings` → `b.d.Settings`, а в `jobRow` — `time.Unix(it.UpdatedAt, 0).In(b.loc).Format("02.01.2006 15:04")` → `b.formatTime(it.UpdatedAt)` (импорт `time` из `jobs.go` убрать).

`internal/screens/home.go`:
```go
// Файл: internal/screens/home.go
package screens

import (
	"context"
	"fmt"

	jobsdomain "pylesos/internal/jobs/domain"
	sourcesdomain "pylesos/internal/sources/domain"
	"pylesos/internal/texts"
)

// Home — главная: плашки «ждёт вас» и группы Telegram.
func (b *Builder) Home(ctx context.Context) (HomeScreen, error) {
	banners, err := b.banners(ctx)
	if err != nil {
		return HomeScreen{}, err
	}
	rows, err := b.telegramRows(ctx)
	if err != nil {
		return HomeScreen{}, fmt.Errorf("экран «Источники»: %w", err)
	}
	empty := texts.HomeEmpty
	if len(rows) > 0 {
		empty = ""
	}
	return HomeScreen{Title: texts.HomeTitle, Banners: banners, EmptyText: empty,
		TelegramTitle: texts.HomeTelegramTitle, Telegram: rows}, nil
}

func (b *Builder) banners(ctx context.Context) ([]Banner, error) {
	items, err := b.d.Jobs.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("экран «Источники»: %w", err)
	}
	banners := []Banner{}
	for _, it := range items {
		if it.Status == jobsdomain.StatusWaitingUser {
			text := fmt.Sprintf(texts.BannerWaiting, it.SourceTitle, b.d.Kinds.KindTitle(it.Kind), it.Reason)
			banners = append(banners, Banner{JobID: it.ID, Text: text})
		}
	}
	return banners, nil
}

func (b *Builder) telegramRows(ctx context.Context) ([]SourceRow, error) {
	sources, err := b.d.Sources.ListByKind(ctx, sourcesdomain.KindTelegramGroup)
	if err != nil {
		return nil, err
	}
	counts, err := b.d.Counts.Counts(ctx)
	if err != nil {
		return nil, err
	}
	stats, err := b.d.Stats.SourceStats(ctx)
	if err != nil {
		return nil, err
	}
	rows := make([]SourceRow, 0, len(sources))
	for _, s := range sources {
		rows = append(rows, b.sourceRow(s, counts[s.ID], stats[s.ID]))
	}
	return rows, nil
}

func (b *Builder) sourceRow(s sourcesdomain.Source, count int, st jobsdomain.SourceStats) SourceRow {
	last := texts.TgNeverSynced
	if st.LastDone > 0 {
		last = fmt.Sprintf(texts.TgLastSync, b.formatTime(st.LastDone))
	}
	actions := []Action{}
	if !st.Active {
		actions = append(actions, Action{ID: "collect", Label: texts.TgCollect, Tone: "primary"})
	}
	return SourceRow{ID: s.ID, Title: s.Title, Lines: []string{fmt.Sprintf(texts.TgMessagesCount, count), last}, Actions: actions}
}
```
(старую функцию `Home` из `builder.go` удалить — она заменена этой.)

`internal/screens/telegram.go`:
```go
// Файл: internal/screens/telegram.go
package screens

import (
	"context"
	"errors"
	"fmt"

	tgservice "pylesos/internal/telegram/service"
	"pylesos/internal/texts"
)

func (b *Builder) Telegram(ctx context.Context) (TelegramScreen, error) {
	return b.telegram(ctx, nil, nil, "", false)
}

// TelegramSubmit — шаг входа; при ошибках форма возвращается с введённым (кроме пароля) и ошибками у полей.
func (b *Builder) TelegramSubmit(ctx context.Context, values map[string]string) (TelegramScreen, error) {
	problems, err := b.d.Login.Submit(ctx, values)
	if err != nil {
		return TelegramScreen{}, fmt.Errorf("вход в Telegram: %w", err)
	}
	if len(problems) > 0 {
		return b.telegram(ctx, values, problems, "", false)
	}
	return b.telegram(ctx, nil, nil, "", false)
}

func (b *Builder) TelegramAddGroup(ctx context.Context, key string) (TelegramScreen, error) {
	err := b.d.Groups.Add(ctx, key)
	if errors.Is(err, tgservice.ErrGroupGone) {
		return b.telegram(ctx, nil, nil, texts.ErrTgGroupGone, false)
	}
	if err != nil {
		return TelegramScreen{}, fmt.Errorf("добавить группу: %w", err)
	}
	return b.telegram(ctx, nil, nil, texts.TgGroupAdded, false)
}

func (b *Builder) TelegramRefresh(ctx context.Context) (TelegramScreen, error) {
	return b.telegram(ctx, nil, nil, "", true)
}

func (b *Builder) telegram(ctx context.Context, input, problems map[string]string, message string, refresh bool) (TelegramScreen, error) {
	st, err := b.d.Login.State(ctx)
	if err != nil {
		return TelegramScreen{}, fmt.Errorf("состояние входа в Telegram: %w", err)
	}
	s := TelegramScreen{Title: texts.TgTitle, SubmitLabel: texts.TgNext, SavingLabel: texts.Saving, Message: message,
		GroupsTitle: texts.TgGroupsTitle, RefreshLabel: texts.TgRefresh, Fields: []Field{}, Groups: []GroupRow{}}
	if st.Step != tgservice.StepReady {
		s.Hint, s.Fields = loginStep(st, input, problems)
		return s, nil
	}
	s.Status = fmt.Sprintf(texts.TgLoggedIn, st.Name)
	views, err := b.d.Groups.List(ctx, refresh)
	if err != nil {
		return TelegramScreen{}, fmt.Errorf("группы Telegram: %w", err)
	}
	if len(views) == 0 {
		s.GroupsEmpty = texts.TgGroupsEmpty
	}
	for _, v := range views {
		s.Groups = append(s.Groups, groupRow(v))
	}
	return s, nil
}

func loginStep(st tgservice.LoginState, input, problems map[string]string) (string, []Field) {
	field := func(key, label, kind string) Field {
		value := input[key]
		if kind == "password" {
			value = "" // пароль в форму не возвращается
		}
		return Field{Key: key, Label: label, Value: value, Kind: kind, Error: problems[key]}
	}
	switch st.Step {
	case tgservice.StepKeys:
		return texts.TgKeysHint, []Field{field(tgservice.FieldAPIID, texts.TgAPIID, "number"),
			field(tgservice.FieldAPIHash, texts.TgAPIHash, "text")}
	case tgservice.StepCode:
		return fmt.Sprintf(texts.TgCodeHint, st.Phone), []Field{field(tgservice.FieldCode, texts.TgCode, "text")}
	case tgservice.StepPassword:
		return texts.TgPasswordHint, []Field{field(tgservice.FieldPassword, texts.TgPassword, "password")}
	default:
		return texts.TgPhoneHint, []Field{field(tgservice.FieldPhone, texts.TgPhone, "text")}
	}
}

func groupRow(v tgservice.GroupView) GroupRow {
	row := GroupRow{Key: v.Group.Peer.Key(), Title: v.Group.Title}
	if v.Added {
		row.Status = texts.TgAdded
		return row
	}
	row.Action = &Action{ID: "add", Label: texts.TgAddGroup, Tone: "primary"}
	return row
}
```

- [ ] **Step 4: Запустить — должен пройти**

Run: `go test ./internal/screens/ ./internal/jobs/...`
Expected: PASS (включая тесты M1).

- [ ] **Step 5: Проверка**

Run: `go vet ./... && go test ./...`
Expected: пакет `cmd/pylesos` не собирается из-за новой сигнатуры `NewBuilder` — это ожидаемо, чинится в Task 10. Остальные пакеты — PASS. Если `go vet` падает только на `cmd/pylesos` — продолжать.

---

### Task 8: HTTP — вход, группы, «Собрать»

**Files:**
- Modify: `internal/api/server.go` (порты и маршруты), Create: `internal/api/handlers_telegram.go`
- Modify/Test: `internal/api/api_test.go`, Create test: `internal/api/telegram_test.go`

**Interfaces:**
- Consumes: `screens.TelegramScreen`, `screens.HomeScreen` (Task 7), `jobsdomain.ErrSourceBusy`
- Produces:
  - `api.Screens` дополняется: `Telegram(ctx)`, `TelegramSubmit(ctx, map[string]string)`, `TelegramAddGroup(ctx, key string)`, `TelegramRefresh(ctx)` → `(screens.TelegramScreen, error)`
  - `api.Collector{Collect(ctx, sourceID int64) error}`; `api.Deps.Collector`
  - Маршруты: `GET /api/screens/telegram`; `POST /api/telegram/submit` (`{"values":{...}}`); `POST /api/telegram/groups` (`{"key":"..."}`); `POST /api/telegram/refresh`; `POST /api/sources/{id}/collect` → `HomeScreen`; занятый источник → 409 `texts.ErrSourceBusy`

- [ ] **Step 1: Написать падающие тесты**

В `internal/api/api_test.go` в `fakeScreens` добавить методы Telegram, а в `start` — `Collector`:
```go
func (f *fakeScreens) Telegram(context.Context) (screens.TelegramScreen, error) {
	return screens.TelegramScreen{Title: texts.TgTitle}, nil
}
func (f *fakeScreens) TelegramSubmit(_ context.Context, v map[string]string) (screens.TelegramScreen, error) {
	f.saved = v
	return screens.TelegramScreen{Title: texts.TgTitle}, nil
}
func (f *fakeScreens) TelegramAddGroup(_ context.Context, key string) (screens.TelegramScreen, error) {
	f.saved = map[string]string{"key": key}
	return screens.TelegramScreen{Message: texts.TgGroupAdded}, nil
}
func (f *fakeScreens) TelegramRefresh(context.Context) (screens.TelegramScreen, error) {
	return screens.TelegramScreen{Title: texts.TgTitle}, nil
}

type fakeCollector struct{ err error }

func (f fakeCollector) Collect(context.Context, int64) error { return f.err }
```
и в `New(Deps{...})` внутри `start` добавить `Collector: fakeCollector{}`; для теста занятости — отдельная функция (ниже). В `shutdown_test.go` в `New(Deps{...})` тоже добавить `Collector: fakeCollector{}`.

`internal/api/telegram_test.go`:
```go
// Файл: internal/api/telegram_test.go
package api

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"pylesos/internal/events"
	jobsdomain "pylesos/internal/jobs/domain"
	"pylesos/internal/screens"
	"pylesos/internal/texts"
)

func TestTelegramRoutes(t *testing.T) {
	_, ts, fs, _ := start(t, fakeJobs{})
	if resp := do(t, http.MethodGet, ts.URL+"/api/screens/telegram", "секрет", ""); resp.StatusCode != http.StatusOK {
		t.Fatalf("экран: %d", resp.StatusCode)
	}
	do(t, http.MethodPost, ts.URL+"/api/telegram/submit", "секрет", `{"values":{"phone":"+77011234567"}}`)
	if fs.saved["phone"] != "+77011234567" {
		t.Fatalf("шаг входа не передан: %v", fs.saved)
	}
	resp := do(t, http.MethodPost, ts.URL+"/api/telegram/groups", "секрет", `{"key":"channel:1:2"}`)
	var s screens.TelegramScreen
	if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
		t.Fatal(err)
	}
	if fs.saved["key"] != "channel:1:2" || s.Message != texts.TgGroupAdded {
		t.Fatalf("добавление группы: %v %+v", fs.saved, s)
	}
	if resp := do(t, http.MethodPost, ts.URL+"/api/telegram/refresh", "секрет", `{}`); resp.StatusCode != http.StatusOK {
		t.Fatalf("обновление: %d", resp.StatusCode)
	}
	if resp := do(t, http.MethodPost, ts.URL+"/api/sources/5/collect", "секрет", `{}`); resp.StatusCode != http.StatusOK {
		t.Fatalf("собрать: %d", resp.StatusCode)
	}
}

func TestCollectBusySource(t *testing.T) {
	s, err := New(Deps{Token: "секрет", Static: fstest.MapFS{}, Bus: events.NewBus(), Screens: &fakeScreens{},
		Jobs: fakeJobs{}, Collector: fakeCollector{err: jobsdomain.ErrSourceBusy}, Log: slog.New(slog.NewJSONHandler(io.Discard, nil))})
	if err != nil {
		t.Fatal(err)
	}
	ln, err := s.Listen()
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewUnstartedServer(s.Handler())
	ts.Listener = ln
	ts.Start()
	defer ts.Close()
	resp := do(t, http.MethodPost, ts.URL+"/api/sources/5/collect", "секрет", `{}`)
	var body struct{ Error string }
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusConflict || body.Error != texts.ErrSourceBusy {
		t.Fatalf("%d %q", resp.StatusCode, body.Error)
	}
}
```

Run: `go test ./internal/api/`
Expected: FAIL — `unknown field Collector`, методы не в интерфейсе/маршрутах.

- [ ] **Step 2: Реализовать**

В `internal/api/server.go`: в интерфейс `Screens` добавить
```go
	Telegram(ctx context.Context) (screens.TelegramScreen, error)
	TelegramSubmit(ctx context.Context, values map[string]string) (screens.TelegramScreen, error)
	TelegramAddGroup(ctx context.Context, key string) (screens.TelegramScreen, error)
	TelegramRefresh(ctx context.Context) (screens.TelegramScreen, error)
```
добавить порт
```go
// Collector — кнопка «Собрать» у источника.
type Collector interface {
	Collect(ctx context.Context, sourceID int64) error
}
```
в `Deps` — поле `Collector Collector`; в `Handler()` — маршруты
```go
	mux.HandleFunc("POST /api/telegram/submit", s.handleTelegramSubmit)
	mux.HandleFunc("POST /api/telegram/groups", s.handleTelegramGroup)
	mux.HandleFunc("POST /api/telegram/refresh", s.handleTelegramRefresh)
	mux.HandleFunc("POST /api/sources/{id}/collect", s.handleCollect)
```
В `handleScreen` (`internal/api/handlers.go`) добавить ветку:
```go
	case "telegram":
		body, err = s.deps.Screens.Telegram(r.Context())
```

`internal/api/handlers_telegram.go`:
```go
// Файл: internal/api/handlers_telegram.go
package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	jobsdomain "pylesos/internal/jobs/domain"
	"pylesos/internal/texts"
)

func (s *Server) handleTelegramSubmit(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Values map[string]string `json:"values"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, texts.ErrBadRequest)
		return
	}
	screen, err := s.deps.Screens.TelegramSubmit(r.Context(), req.Values)
	s.writeScreen(w, "вход в Telegram", screen, err)
}

func (s *Server) handleTelegramGroup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Key string `json:"key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, texts.ErrBadRequest)
		return
	}
	screen, err := s.deps.Screens.TelegramAddGroup(r.Context(), req.Key)
	s.writeScreen(w, "добавить группу", screen, err)
}

func (s *Server) handleTelegramRefresh(w http.ResponseWriter, r *http.Request) {
	screen, err := s.deps.Screens.TelegramRefresh(r.Context())
	s.writeScreen(w, "обновить группы", screen, err)
}

// handleCollect — «Собрать»; в ответ — обновлённая главная.
func (s *Server) handleCollect(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, texts.ErrBadRequest)
		return
	}
	if err := s.deps.Collector.Collect(r.Context(), id); err != nil {
		if errors.Is(err, jobsdomain.ErrSourceBusy) {
			s.writeError(w, http.StatusConflict, texts.ErrSourceBusy)
			return
		}
		s.deps.Log.Error("собрать источник", "source", id, "err", err)
		s.writeError(w, http.StatusInternalServerError, texts.ErrInternal)
		return
	}
	screen, err := s.deps.Screens.Home(r.Context())
	s.writeScreen(w, "главная", screen, err)
}

// writeScreen отдаёт экран или логирует ошибку и отвечает текстом для пользователя.
func (s *Server) writeScreen(w http.ResponseWriter, what string, screen any, err error) {
	if err != nil {
		s.deps.Log.Error(what, "err", err)
		s.writeError(w, http.StatusInternalServerError, texts.ErrInternal)
		return
	}
	s.writeJSON(w, http.StatusOK, screen)
}
```

- [ ] **Step 3: Запустить — должен пройти**

Run: `go test -race ./internal/api/`
Expected: PASS.

- [ ] **Step 4: Проверка**

Run: `go vet ./internal/... && go test ./internal/...`
Expected: без ошибок (`cmd/pylesos` чинится в Task 10).

---

### Task 9: Фронт — вкладка «Telegram», группы на главной

**Files:**
- Modify: `web/src/types.ts`, `web/src/events.ts`, `web/src/App.tsx`, `web/src/pages/HomePage.tsx`
- Create: `web/src/pages/TelegramPage.tsx`

**Interfaces:**
- Consumes: JSON `HomeScreen`, `TelegramScreen` (Task 7), маршруты Task 8
- Produces: вкладка `telegram`; кнопка «Собрать»; события `jobs_changed` перечитывают только «Источники» и «Задания» (отложенная мелочь №9 из ревью M1 — формы больше не сбрасываются событиями)

- [ ] **Step 1: Типы и события**

В `web/src/types.ts`: заменить `Field` и `HomeScreen`, добавить новые типы:
```ts
export interface Field { key: string; label: string; value: string; kind: 'text' | 'number' | 'password'; error: string }
export interface SourceRow { id: number; title: string; lines: string[]; actions: Action[] }
export interface HomeScreen { title: string; banners: Banner[]; empty_text: string; telegram_title: string; telegram: SourceRow[] }
export interface GroupRow { key: string; title: string; status: string; action: Action | null }
export interface TelegramScreen {
  title: string
  hint: string
  fields: Field[]
  submit_label: string
  saving_label: string
  status: string
  message: string
  groups_title: string
  groups_empty: string
  refresh_label: string
  groups: GroupRow[]
}
```

В `web/src/events.ts` заменить обработчик:
```ts
    source.onmessage = () => {
      // jobs_changed: меняются только сводки; формы на других экранах не трогаем.
      void queryClient.invalidateQueries({ queryKey: ['screen', 'jobs'] })
      void queryClient.invalidateQueries({ queryKey: ['screen', 'home'] })
    }
```

- [ ] **Step 2: Страница «Telegram»**

`web/src/pages/TelegramPage.tsx`:
```tsx
// Файл: web/src/pages/TelegramPage.tsx
// Вход по шагам и группы аккаунта; шаг, поля, ошибки и кнопки приходят с бэкенда.
import { useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { postJSON } from '../api'
import { ScreenView } from '../components/ScreenView'
import { Button } from '../components/ui/button'
import { screenKey, useScreen } from '../queries'
import type { TelegramScreen } from '../types'

function useTelegramAction<V>(path: (v: V) => string, body: (v: V) => unknown) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (v: V) => postJSON<TelegramScreen>(path(v), body(v)),
    onSuccess: (next) => queryClient.setQueryData(screenKey('telegram'), next),
  })
}

function LoginForm({ screen }: { screen: TelegramScreen }) {
  const [values, setValues] = useState<Record<string, string>>(() =>
    Object.fromEntries(screen.fields.map((f) => [f.key, f.value])),
  )
  const submit = useTelegramAction(() => '/api/telegram/submit', (v: Record<string, string>) => ({ values: v }))
  return (
    <form className="max-w-xl space-y-4" onSubmit={(e) => { e.preventDefault(); submit.mutate(values) }}>
      <p>{screen.hint}</p>
      {screen.fields.map((f) => (
        <label key={f.key} className="block space-y-1">
          <span className="text-sm">{f.label}</span>
          <input
            type={f.kind}
            value={values[f.key] ?? ''}
            onChange={(e) => setValues({ ...values, [f.key]: e.target.value })}
            className="w-full rounded-md border border-neutral-300 bg-transparent px-2 py-1 dark:border-neutral-700"
          />
          {f.error && <span className="text-sm text-red-600">{f.error}</span>}
        </label>
      ))}
      {submit.error && <p className="text-red-600">{submit.error.message}</p>}
      <Button type="submit" disabled={submit.isPending}>
        {submit.isPending ? screen.saving_label : screen.submit_label}
      </Button>
    </form>
  )
}

function Groups({ screen }: { screen: TelegramScreen }) {
  const add = useTelegramAction(() => '/api/telegram/groups', (key: string) => ({ key }))
  const refresh = useTelegramAction(() => '/api/telegram/refresh', () => ({}))
  return (
    <section className="space-y-3">
      <p className="text-green-600">{screen.status}</p>
      <div className="flex items-center gap-2">
        <h2 className="font-semibold">{screen.groups_title}</h2>
        <Button variant="outline" disabled={refresh.isPending} onClick={() => refresh.mutate(undefined)}>
          {screen.refresh_label}
        </Button>
      </div>
      {(add.error ?? refresh.error) && <p className="text-red-600">{(add.error ?? refresh.error)?.message}</p>}
      {screen.groups_empty && <p className="text-neutral-500">{screen.groups_empty}</p>}
      {screen.groups.map((g) => (
        <div key={g.key} className="flex items-center gap-2 rounded-md border border-neutral-200 p-2 dark:border-neutral-800">
          <span>{g.title}</span>
          <span className="ml-auto text-sm text-neutral-500">{g.status}</span>
          {g.action && (
            <Button disabled={add.isPending} onClick={() => add.mutate(g.key)}>{g.action.label}</Button>
          )}
        </div>
      ))}
    </section>
  )
}

export function TelegramPage() {
  const query = useScreen<TelegramScreen>('telegram')
  return (
    <ScreenView query={query}>
      {(s) => (
        <section className="space-y-4">
          <h1 className="text-xl font-semibold">{s.title}</h1>
          {s.message && <p className="text-amber-700 dark:text-amber-400">{s.message}</p>}
          {s.fields.length > 0 ? <LoginForm key={s.hint} screen={s} /> : <Groups screen={s} />}
        </section>
      )}
    </ScreenView>
  )
}
```

- [ ] **Step 3: Главная и меню**

`web/src/pages/HomePage.tsx` (целиком):
```tsx
// Файл: web/src/pages/HomePage.tsx
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { postJSON } from '../api'
import { ScreenView } from '../components/ScreenView'
import { Button } from '../components/ui/button'
import { screenKey, useScreen } from '../queries'
import type { HomeScreen } from '../types'

export function HomePage() {
  const query = useScreen<HomeScreen>('home')
  const queryClient = useQueryClient()
  const collect = useMutation({
    mutationFn: (id: number) => postJSON<HomeScreen>(`/api/sources/${id}/collect`, {}),
    onSuccess: (next) => queryClient.setQueryData(screenKey('home'), next),
  })
  return (
    <ScreenView query={query}>
      {(s) => (
        <section className="space-y-3">
          <h1 className="text-xl font-semibold">{s.title}</h1>
          {s.banners.map((b) => (
            <div key={b.job_id} className="rounded-md border border-amber-400 bg-amber-50 p-3 dark:bg-amber-950">
              {b.text}
            </div>
          ))}
          {s.empty_text && <p className="text-neutral-500">{s.empty_text}</p>}
          {collect.error && <p className="text-red-600">{collect.error.message}</p>}
          {s.telegram.length > 0 && <h2 className="font-semibold">{s.telegram_title}</h2>}
          {s.telegram.map((row) => (
            <div key={row.id} className="flex items-center gap-3 rounded-md border border-neutral-200 p-3 dark:border-neutral-800">
              <div className="space-y-0.5">
                <div className="font-medium">{row.title}</div>
                {row.lines.map((line) => <div key={line} className="text-sm text-neutral-500">{line}</div>)}
              </div>
              <div className="ml-auto flex gap-2">
                {row.actions.map((a) => (
                  <Button key={a.id} disabled={collect.isPending} onClick={() => collect.mutate(row.id)}>{a.label}</Button>
                ))}
              </div>
            </div>
          ))}
        </section>
      )}
    </ScreenView>
  )
}
```

В `web/src/App.tsx`: импорт `import { TelegramPage } from './pages/TelegramPage'` и в `pages` добавить `telegram: TelegramPage`.

- [ ] **Step 4: Проверить типы и сборку**

Run: `cd web && npm run typecheck && npm run build && touch dist/.keep`
Expected: без ошибок TypeScript; сборка есть.

---

### Task 10: Сборка зависимостей и запуск

**Files:**
- Modify: `cmd/pylesos/wire.go`, Create: `cmd/pylesos/telegram.go`

**Interfaces:**
- Consumes: всё из задач 1–9
- Produces: рабочая программа: вкладка «Telegram», сбор, догрузка при запуске

- [ ] **Step 1: Модуль Telegram в сборке**

`cmd/pylesos/telegram.go`:
```go
// Файл: cmd/pylesos/telegram.go
package main

import (
	"context"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/jmoiron/sqlx"

	"pylesos/internal/paths"
	"pylesos/internal/secrets"
	sourcesrepo "pylesos/internal/sources/repository"
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
	worker := tgservice.NewSyncWorker(gw, msgs, sources, time.Now, tgservice.SleepCtx, 100)
	return telegramParts{gateway: gw, sources: sources, msgs: msgs, keys: keys, worker: worker}, nil
}
```

`cmd/pylesos/wire.go` (целиком):
```go
// Файл: cmd/pylesos/wire.go
package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/jmoiron/sqlx"

	"pylesos/internal/api"
	"pylesos/internal/events"
	jobsrepo "pylesos/internal/jobs/repository"
	jobsservice "pylesos/internal/jobs/service"
	"pylesos/internal/paths"
	"pylesos/internal/screens"
	settingsdomain "pylesos/internal/settings/domain"
	settingsrepo "pylesos/internal/settings/repository"
	settingsservice "pylesos/internal/settings/service"
	tgservice "pylesos/internal/telegram/service"
	"pylesos/web"
)

type app struct {
	server *api.Server
	runner *jobsservice.Runner
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
	runner := jobsservice.NewRunner(ctx, jobs, bus, log, time.Now, tg.worker)
	if err := runner.ResumeInterrupted(ctx); err != nil {
		return app{}, err
	}
	login := tgservice.NewLogin(tg.gateway, tg.keys)
	groups := tgservice.NewGroups(tg.gateway, tg.sources, runner, time.Now)
	go syncAtStart(ctx, groups, log)
	static, err := web.Dist()
	if err != nil {
		return app{}, err
	}
	builder := screens.NewBuilder(screens.Deps{Jobs: jobs, Kinds: runner, Settings: settings, Sources: tg.sources,
		Counts: tg.msgs, Stats: jobs, Login: login, Groups: groups, Loc: time.Local})
	server, err := api.New(api.Deps{Static: static, Bus: bus, Screens: builder, Jobs: runner, Collector: groups, Log: log})
	if err != nil {
		return app{}, err
	}
	return app{server: server, runner: runner}, nil
}

// syncAtStart догружает группы при запуске (спецификация, раздел 3, п. 4); сеть не задерживает окно.
func syncAtStart(ctx context.Context, groups *tgservice.Groups, log *slog.Logger) {
	if err := groups.SyncAll(ctx); err != nil && ctx.Err() == nil {
		log.Warn("догрузка групп при запуске", "err", err)
	}
}
```

- [ ] **Step 2: Полная сборка**

Run: `go mod tidy && scripts/build.sh`
Expected: последняя строка `Готово: bin/pylesos`; `go vet` и `go test` без ошибок.

- [ ] **Step 3: Итоговая проверка**

Run: `go test -race ./... && GOOS=windows go vet ./... && GOOS=darwin go vet ./...`
Expected: все пакеты PASS; сборка под Windows/macOS без ошибок. Вход с реальным аккаунтом Telegram — только по просьбе пользователя.

- [ ] **Step 4: База знаний**

Спросить пользователя про обновление базы знаний: `docs/CURRENT_STATE.md`, `road_map.md` (M2 ✅), `08-development-stages.md`, `docs/reference/db-schema.md` (`secrets`, `sources_kind_config`, `tg_messages`, `tg_media`), `api-routes.md`, `domain-models.md` (фазы `tg_sync`), грабли в `03-telegram.md` (COALESCE сообщений, FLOOD_WAIT в прогрессе).
