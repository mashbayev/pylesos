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
