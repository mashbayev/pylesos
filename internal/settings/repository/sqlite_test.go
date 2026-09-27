// Файл: internal/settings/repository/sqlite_test.go
package repository

import (
	"context"
	"testing"

	"pylesos/internal/store/storetest"
)

func TestPutAndAll(t *testing.T) {
	ctx := context.Background()
	repo := NewSQLite(storetest.Open(t))
	if err := repo.Put(ctx, map[string]string{"a": "1", "b": "2"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.Put(ctx, map[string]string{"a": "3"}); err != nil {
		t.Fatal(err)
	}
	got, err := repo.All(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got["a"] != "3" || got["b"] != "2" || len(got) != 2 {
		t.Fatalf("настройки: %v", got)
	}
}
