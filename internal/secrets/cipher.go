// Файл: internal/secrets/cipher.go
// Package secrets — шифрованное хранение секретов (ключи API и сессия Telegram): AES-256-GCM (10-security.md).
package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const keySize = 32

var (
	ErrBadKey    = errors.New("файл ключа шифрования повреждён")
	ErrCorrupted = errors.New("секрет повреждён или зашифрован другим ключом")
)

// LoadOrCreateKey читает ключ шифрования; файла нет — создаёт случайный с правами 0600.
func LoadOrCreateKey(path string) ([]byte, error) {
	key, err := os.ReadFile(path)
	if err == nil {
		if len(key) != keySize {
			return nil, ErrBadKey
		}
		return key, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("прочитать ключ шифрования: %w", err)
	}
	key = make([]byte, keySize)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("создать ключ шифрования: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("папка ключа шифрования: %w", err)
	}
	if err := os.WriteFile(path, key, 0o600); err != nil {
		return nil, fmt.Errorf("записать ключ шифрования: %w", err)
	}
	return key, nil
}

type Cipher struct{ aead cipher.AEAD }

func NewCipher(key []byte) (*Cipher, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("ключ шифрования: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("режим GCM: %w", err)
	}
	return &Cipher{aead: aead}, nil
}

// Seal шифрует; результат — nonce и шифротекст.
func (c *Cipher) Seal(plain []byte) ([]byte, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("nonce: %w", err)
	}
	return c.aead.Seal(nonce, nonce, plain, nil), nil
}

// Open расшифровывает; повреждённые данные или чужой ключ — ErrCorrupted.
func (c *Cipher) Open(sealed []byte) ([]byte, error) {
	n := c.aead.NonceSize()
	if len(sealed) < n {
		return nil, ErrCorrupted
	}
	plain, err := c.aead.Open(nil, sealed[:n], sealed[n:], nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCorrupted, err)
	}
	return plain, nil
}
