// Файл: internal/settings/service/service_test.go
package service

import (
	"context"
	"fmt"
	"testing"

	"pylesos/internal/settings/domain"
	"pylesos/internal/texts"
)

type fakeRepo struct{ data map[string]string }

func (f *fakeRepo) All(context.Context) (map[string]string, error) {
	out := make(map[string]string, len(f.data))
	for k, v := range f.data {
		out[k] = v
	}
	return out, nil
}

func (f *fakeRepo) Put(_ context.Context, values map[string]string) error {
	for k, v := range values {
		f.data[k] = v
	}
	return nil
}

var defaults = domain.Settings{ExportsDir: "/home/u/Документы/Пылесос", PhonePauseSec: domain.DefaultPhonePauseSec}

func TestGetDefaultsAndBadStoredValue(t *testing.T) {
	svc := NewService(&fakeRepo{data: map[string]string{domain.KeyPhonePauseSec: "abc"}}, defaults)
	got, err := svc.Get(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got != defaults {
		t.Fatalf("got %+v, want %+v", got, defaults)
	}
}

func TestSaveValid(t *testing.T) {
	repo := &fakeRepo{data: map[string]string{}}
	svc := NewService(repo, defaults)
	saved, problems, err := svc.Save(context.Background(), map[string]string{
		domain.KeyExportsDir: " /data/выгрузки/ ", domain.KeyPhonePauseSec: " 45 ",
	})
	if err != nil || len(problems) != 0 {
		t.Fatalf("err=%v problems=%v", err, problems)
	}
	if saved.ExportsDir != "/data/выгрузки" || saved.PhonePauseSec != 45 {
		t.Fatalf("сохранено: %+v", saved)
	}
}

func TestSaveRejectsTildeAndRelative(t *testing.T) {
	for _, dir := range []string{"~/Документы", "Документы/Пылесос", "  "} {
		repo := &fakeRepo{data: map[string]string{}}
		svc := NewService(repo, defaults)
		_, problems, err := svc.Save(context.Background(), map[string]string{
			domain.KeyExportsDir: dir, domain.KeyPhonePauseSec: "30",
		})
		if err != nil {
			t.Fatal(err)
		}
		if problems[domain.KeyExportsDir] == "" {
			t.Fatalf("%q: нет ошибки у поля", dir)
		}
		if len(repo.data) != 0 {
			t.Fatalf("%q: при ошибке что-то сохранилось: %v", dir, repo.data)
		}
	}
}

func TestSavePauseErrors(t *testing.T) {
	cases := map[string]string{
		"пять": texts.ErrPauseNotNumber,
		"4":    fmt.Sprintf(texts.ErrPauseRange, domain.MinPhonePauseSec, domain.MaxPhonePauseSec),
		"601":  fmt.Sprintf(texts.ErrPauseRange, domain.MinPhonePauseSec, domain.MaxPhonePauseSec),
		"30.5": texts.ErrPauseNotNumber,
	}
	for input, want := range cases {
		svc := NewService(&fakeRepo{data: map[string]string{}}, defaults)
		_, problems, err := svc.Save(context.Background(), map[string]string{
			domain.KeyExportsDir: "/data", domain.KeyPhonePauseSec: input,
		})
		if err != nil {
			t.Fatal(err)
		}
		if problems[domain.KeyPhonePauseSec] != want {
			t.Fatalf("%q: got %q, want %q", input, problems[domain.KeyPhonePauseSec], want)
		}
	}
}
