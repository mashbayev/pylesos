// Файл: internal/people/service/service_test.go
package service

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"pylesos/internal/people/domain"
	"pylesos/internal/people/repository"
	"pylesos/internal/store/storetest"
)

func TestMergeKeepsPersonWithIIN(t *testing.T) {
	ctx := context.Background()
	repo := repository.NewSQLite(storetest.Open(t))
	a, err := repo.CreatePerson(ctx, domain.Person{FullName: "Тестов Иван", NameKey: "иван тестов"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := repo.CreatePerson(ctx, domain.Person{FullName: "Тестов Иван Петрович", NameKey: "иван петрович тестов",
		IIN: sql.NullString{String: "900101300007", Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	s := New(repo, func() time.Time { return time.Unix(1, 0) })
	if err := s.Merge(ctx, a, b); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Person(ctx, a); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("остаться должен человек с ИИН: %v", err)
	}
	if err := s.Link(ctx, 1, 0, " ЧС "); !errors.Is(err, ErrNoName) {
		t.Fatalf("новый человек без имени: %v", err)
	}
}
