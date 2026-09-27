// Файл: internal/sources/repository/sqlite_test.go
package repository

import (
	"context"
	"errors"
	"testing"

	"pylesos/internal/sources/domain"
	"pylesos/internal/store/storetest"
)

func TestEnsureIsIdempotentPerConfig(t *testing.T) {
	ctx := context.Background()
	repo := NewSQLite(storetest.Open(t))
	a := domain.Source{Kind: domain.KindTelegramGroup, Title: "Группа А", Config: `{"id":1}`, CreatedAt: 100}
	id, created, err := repo.Ensure(ctx, a)
	if err != nil || !created {
		t.Fatalf("создание: id=%d created=%v err=%v", id, created, err)
	}
	again, created, err := repo.Ensure(ctx, a)
	if err != nil || created || again != id {
		t.Fatalf("повтор: id=%d created=%v err=%v", again, created, err)
	}
	b := domain.Source{Kind: domain.KindTelegramGroup, Title: "Группа Б", Config: `{"id":2}`, CreatedAt: 100}
	other, created, err := repo.Ensure(ctx, b)
	if err != nil || !created || other == id {
		t.Fatalf("другая группа: id=%d created=%v err=%v", other, created, err)
	}
	list, err := repo.ListByKind(ctx, domain.KindTelegramGroup)
	if err != nil || len(list) != 2 || list[0].Title != "Группа А" {
		t.Fatalf("список: %+v %v", list, err)
	}
	sites, err := repo.ListByKind(ctx, domain.KindSite)
	if err != nil || sites == nil || len(sites) != 0 {
		t.Fatalf("пустой список должен быть []: %#v %v", sites, err)
	}
	got, err := repo.Get(ctx, other)
	if err != nil || got.Config != `{"id":2}` {
		t.Fatalf("Get: %+v %v", got, err)
	}
	if _, err := repo.Get(ctx, 999); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("нет источника: %v", err)
	}
}
