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
