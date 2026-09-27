// Файл: internal/analyze/mentions_test.go
package analyze

import (
	"context"
	"errors"
	"strings"
	"testing"

	tgdomain "pylesos/internal/telegram/domain"
)

func TestFindSendsContextAndParses(t *testing.T) {
	f := &fakeLLM{mentions: map[string]string{"Тестов Иван": mentionsJSON(t, Found{Name: "Тестов Иван", Role: "record"}, Found{Name: "Кто-то", Role: "непонятно"})}}
	msg := tgdomain.Message{ID: 5, Text: "Тестов Иван ЧС! Долг по аренде"}
	docs := []map[string]string{{"full_name": "ТЕСТОВ ИВАН", "iin": "900101300007"}}
	found, err := (&Finder{LLM: f}).Find(context.Background(), "m", msg, docs, "Есть инфо о нём?")
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 || found[0].Name != "Тестов Иван" || found[0].Role != "record" {
		t.Fatalf("упоминания (неизвестная роль отброшена): %+v", found)
	}
	req := f.reqs[0]
	if !strings.Contains(req.Text, "900101300007") || !strings.Contains(req.Text, "Есть инфо о нём?") || len(req.Schema) == 0 || req.Model != "m" {
		t.Fatalf("в запросе нет документа, ответа или схемы: %+v", req)
	}
}

func TestFindRetriesBadAnswerOnce(t *testing.T) {
	f := &fakeLLM{bad: 1, mentions: map[string]string{"x": `{"mentions":[]}`}}
	if _, err := (&Finder{LLM: f}).Find(context.Background(), "m", tgdomain.Message{Text: "x"}, nil, ""); err != nil {
		t.Fatalf("один повтор: %v", err)
	}
	f.bad = 2
	if _, err := (&Finder{LLM: f}).Find(context.Background(), "m", tgdomain.Message{Text: "x"}, nil, ""); !errors.Is(err, ErrBadAnswer) {
		t.Fatalf("дважды мусор: %v", err)
	}
}
