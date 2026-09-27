// Файл: internal/settings/repository/repository.go
// Package repository — хранение настроек.
package repository

import "context"

// Repository хранит настройки как ключ → значение.
type Repository interface {
	All(ctx context.Context) (map[string]string, error)
	Put(ctx context.Context, values map[string]string) error
}
