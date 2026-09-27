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
