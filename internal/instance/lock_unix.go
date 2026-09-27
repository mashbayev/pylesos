// Файл: internal/instance/lock_unix.go
//go:build !windows

package instance

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

// tryLock — flock без ожидания; false — файл заблокирован другим процессом.
func tryLock(f *os.File) (bool, error) {
	err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if errors.Is(err, syscall.EWOULDBLOCK) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("заблокировать файл: %w", err)
	}
	return true, nil
}

func unlock(f *os.File) error {
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_UN); err != nil {
		return fmt.Errorf("снять блокировку: %w", err)
	}
	return nil
}
