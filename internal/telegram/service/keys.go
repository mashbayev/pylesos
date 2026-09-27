// Файл: internal/telegram/service/keys.go
package service

import (
	"context"
	"encoding/json"
	"fmt"
)

const secretAPIKeys = "telegram_api"

// Keys — api_id и api_hash приложения Telegram (вводит пользователь, П19).
type Keys struct {
	AppID   int    `json:"api_id"`
	AppHash string `json:"api_hash"`
}

type KeyStore struct{ secrets SecretStore }

func NewKeyStore(s SecretStore) *KeyStore { return &KeyStore{secrets: s} }

// Load — сохранённые ключи; ok=false — ещё не введены.
func (k *KeyStore) Load(ctx context.Context) (Keys, bool, error) {
	data, ok, err := k.secrets.Get(ctx, secretAPIKeys)
	if err != nil || !ok {
		return Keys{}, false, err
	}
	var keys Keys
	if err := json.Unmarshal(data, &keys); err != nil {
		return Keys{}, false, fmt.Errorf("ключи API Telegram: %w", err)
	}
	return keys, true, nil
}

func (k *KeyStore) Save(ctx context.Context, keys Keys) error {
	data, err := json.Marshal(keys)
	if err != nil {
		return fmt.Errorf("ключи API Telegram: %w", err)
	}
	return k.secrets.Put(ctx, secretAPIKeys, data)
}
