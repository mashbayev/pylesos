// Файл: internal/logger/logger.go
// Package logger — единый JSON-логгер программы (slog).
package logger

import (
	"io"
	"log/slog"
)

// New создаёт JSON-логгер, пишущий в w.
func New(w io.Writer, level slog.Level) *slog.Logger {
	return slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{Level: level}))
}
