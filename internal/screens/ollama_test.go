// Файл: internal/screens/ollama_test.go
package screens

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"pylesos/internal/texts"
)

type fakeOllama struct {
	models []string
	err    error
}

func (f fakeOllama) VisionModels(context.Context) ([]string, error) { return f.models, f.err }

func TestSettingsShowsOllamaStatus(t *testing.T) {
	cases := []struct {
		o    fakeOllama
		want string
	}{
		{fakeOllama{models: []string{"llava:7b", "gemma4:e4b"}}, fmt.Sprintf(texts.OllamaOK, "gemma4:e4b")},
		{fakeOllama{err: errors.New("connection refused")}, texts.OllamaDown},
		{fakeOllama{}, texts.OllamaNoModel},
	}
	for _, c := range cases {
		b := NewBuilder(Deps{Jobs: fakeJobs{}, Kinds: fakeKinds{}, Settings: &fakeSettings{}, Sources: fakeSources{},
			Counts: fakeCounts{}, Stats: fakeStats{}, Login: &fakeLogin{}, Groups: &fakeGroups{}, Bot: &fakeBot{},
			Forms: &fakeForms{}, Ollama: c.o, Loc: time.UTC})
		s, err := b.Settings(context.Background())
		if err != nil || s.OllamaLabel != texts.SettingsOllama || s.OllamaStatus != c.want {
			t.Fatalf("%+v: %q %v", c.o, s.OllamaStatus, err)
		}
	}
}
