// Файл: internal/telegram/gotdgw/errors.go
package gotdgw

import (
	"errors"
	"fmt"

	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/tgerr"

	"pylesos/internal/telegram/domain"
)

// mapErr переводит ошибки Telegram в ошибки domain; остальные — как есть.
func mapErr(err error) error {
	if err == nil {
		return nil
	}
	if d, ok := tgerr.AsFloodWait(err); ok {
		return &domain.FloodWaitError{Wait: d}
	}
	var signUp *auth.SignUpRequired
	switch {
	case errors.Is(err, auth.ErrPasswordAuthNeeded):
		return domain.ErrPasswordNeeded
	case errors.Is(err, auth.ErrPasswordInvalid):
		return fmt.Errorf("%w: %w", domain.ErrBadPassword, err)
	case errors.As(err, &signUp):
		return fmt.Errorf("%w: %w", domain.ErrNoAccount, err)
	case tgerr.Is(err, "API_ID_INVALID", "API_ID_PUBLISHED_FLOOD", "CONNECTION_API_ID_INVALID"):
		return fmt.Errorf("%w: %w", domain.ErrBadKeys, err)
	case tgerr.Is(err, "CHANNEL_PRIVATE", "CHANNEL_INVALID", "CHAT_FORBIDDEN", "CHAT_ID_INVALID", "PEER_ID_INVALID"):
		return fmt.Errorf("%w: %w", domain.ErrGroupUnavailable, err)
	case tgerr.Is(err, "PHONE_NUMBER_INVALID", "PHONE_NUMBER_BANNED", "PHONE_NUMBER_FLOOD"):
		return fmt.Errorf("%w: %w", domain.ErrBadPhone, err)
	case tgerr.Is(err, "PHONE_CODE_INVALID", "PHONE_CODE_EMPTY"):
		return fmt.Errorf("%w: %w", domain.ErrBadCode, err)
	case tgerr.Is(err, "PHONE_CODE_EXPIRED"):
		return fmt.Errorf("%w: %w", domain.ErrCodeExpired, err)
	case tgerr.Is(err, "PASSWORD_HASH_INVALID"):
		return fmt.Errorf("%w: %w", domain.ErrBadPassword, err)
	case tgerr.Is(err, "AUTH_KEY_UNREGISTERED", "SESSION_REVOKED", "SESSION_EXPIRED", "USER_DEACTIVATED"):
		return fmt.Errorf("%w: %w", domain.ErrNotAuthorized, err)
	}
	return err
}
