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
