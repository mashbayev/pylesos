// Файл: internal/instance/lock_test.go
package instance

import (
	"path/filepath"
	"testing"
)

// Вторая копия программы не получает блокировку, пока первая жива.
func TestLockIsExclusive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Пылесос", "instance.lock")
	first, ok, err := Lock(path)
	if err != nil || !ok {
		t.Fatalf("первая блокировка: ok=%v err=%v", ok, err)
	}
	if _, ok, err := Lock(path); err != nil || ok {
		t.Fatalf("вторая блокировка при живой первой: ok=%v err=%v", ok, err)
	}
	if err := first(); err != nil {
		t.Fatal(err)
	}
	again, ok, err := Lock(path)
	if err != nil || !ok {
		t.Fatalf("блокировка после освобождения: ok=%v err=%v", ok, err)
	}
	if err := again(); err != nil {
		t.Fatal(err)
	}
}
