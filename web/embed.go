// Файл: web/embed.go
// Package web встраивает собранный фронт в бинарник (П14).
package web

import (
	"embed"
	"fmt"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// Dist — файлы собранного фронта; корень — содержимое web/dist.
func Dist() (fs.FS, error) {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		return nil, fmt.Errorf("встроенный фронт: %w", err)
	}
	return sub, nil
}
