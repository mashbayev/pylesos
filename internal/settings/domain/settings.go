// Файл: internal/settings/domain/settings.go
// Package domain — настройки программы (экран «Настройки»).
package domain

// Settings — настройки, которые меняет пользователь.
type Settings struct {
	ExportsDir    string // папка выгрузок Excel
	PhonePauseSec int    // пауза между открытиями телефонов OLX (П11)
}

// Ключи в таблице settings.
const (
	KeyExportsDir    = "exports_dir"
	KeyPhonePauseSec = "phone_pause_sec"
)

// Границы паузы между телефонами (спецификация, раздел 3, OLX, п. 4).
const (
	DefaultPhonePauseSec = 30
	MinPhonePauseSec     = 5
	MaxPhonePauseSec     = 600
)
