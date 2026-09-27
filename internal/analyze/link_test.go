// Файл: internal/analyze/link_test.go
package analyze

import (
	"context"
	"testing"
	"time"

	pdomain "pylesos/internal/people/domain"
	prepo "pylesos/internal/people/repository"
	"pylesos/internal/store/storetest"
)

func newLinker(t *testing.T) (*Linker, *prepo.SQLite) {
	t.Helper()
	repo := prepo.NewSQLite(storetest.Open(t))
	return &Linker{People: repo, Now: func() time.Time { return time.Unix(100, 0) }}, repo
}

func TestLinkByIINNameAndDocument(t *testing.T) {
	ctx := context.Background()
	l, repo := newLinker(t)
	byIIN, link, err := l.Link(ctx, Found{Name: "Тестов Иван", IIN: "900101300007", Role: "record"}, nil)
	if err != nil || !byIIN.Valid || link != pdomain.LinkIIN {
		t.Fatalf("по ИИН: %+v %s %v", byIIN, link, err)
	}
	again, link, err := l.Link(ctx, Found{Name: "Иван Тестов", IIN: "900101300007"}, nil)
	if err != nil || again.Int64 != byIIN.Int64 || link != pdomain.LinkIIN {
		t.Fatalf("тот же ИИН — тот же человек: %+v %v", again, err)
	}
	byName, link, err := l.Link(ctx, Found{Name: "ИВАН ТЕСТОВ"}, nil)
	if err != nil || byName.Int64 != byIIN.Int64 || link != pdomain.LinkName {
		t.Fatalf("точное ФИО: %+v %s %v", byName, link, err)
	}
	docs := []map[string]string{{"full_name": "Примеров Пётр", "iin": "910202300008", "doc_number": "N55"}}
	byDoc, link, err := l.Link(ctx, Found{Role: "record"}, docs)
	if err != nil || !byDoc.Valid || link != pdomain.LinkDocument {
		t.Fatalf("по документу: %+v %s %v", byDoc, link, err)
	}
	if p, err := repo.Person(ctx, byDoc.Int64); err != nil || p.DocNumber != "N55" || p.FullName != "Примеров Пётр" {
		t.Fatalf("поля документа: %+v %v", p, err)
	}
}

func TestLinkAmbiguousAndCandidates(t *testing.T) {
	ctx := context.Background()
	l, repo := newLinker(t)
	if _, _, err := l.Link(ctx, Found{Name: "Тестов Иван", IIN: "900101300007"}, nil); err != nil {
		t.Fatal(err)
	}
	if _, _, err := l.Link(ctx, Found{Name: "Тестов Иван", IIN: "920303300009"}, nil); err != nil {
		t.Fatal(err)
	}
	none, link, err := l.Link(ctx, Found{Name: "Тестов Иван"}, nil) // двое с таким ФИО — не угадывать
	if err != nil || none.Valid || link != pdomain.LinkNone {
		t.Fatalf("двое тёзок: %+v %s %v", none, link, err)
	}
	if _, _, err := l.Link(ctx, Found{Name: "Тестов Иван Петрович"}, nil); err != nil {
		t.Fatal(err)
	}
	cands, err := repo.Candidates(ctx)
	if err != nil || len(cands) != 2 {
		t.Fatalf("«возможно, один человек» с обоими тёзками: %+v %v", cands, err)
	}
	if empty, link, err := l.Link(ctx, Found{Name: " ЧС! "}, nil); err != nil || empty.Valid || link != pdomain.LinkNone {
		t.Fatalf("без имени и документа: %+v %s %v", empty, link, err)
	}
}
