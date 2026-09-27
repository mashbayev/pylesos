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
	// History — порция от новых к старым: id < offsetID (0 — с самого нового) и id > minID.
	History(ctx context.Context, p domain.Peer, offsetID, minID, limit int) (domain.Page, error)
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
