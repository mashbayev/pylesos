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
