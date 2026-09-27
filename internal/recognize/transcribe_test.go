// Файл: internal/recognize/transcribe_test.go
package recognize

import (
	"context"
	"errors"
	"strings"
	"testing"

	"pylesos/internal/llm"
	"pylesos/internal/normalize"
)

type fakeCore struct {
	reply string
	err   error
	reqs  []llm.Request
}

func (f *fakeCore) Respond(_ context.Context, r llm.Request) (string, error) {
	f.reqs = append(f.reqs, r)
	return f.reply, f.err
}

var imgPage = normalize.Page{Source: "a.png", Index: 2, Image: []byte{1, 2, 3}}

func TestPageSendsPromptModelAndImage(t *testing.T) {
	fc := &fakeCore{reply: "Қазақстан Республикасы"}
	got, err := (&Transcriber{LLM: fc, Model: "chatgpt-web/high"}).Page(context.Background(), imgPage)
	if err != nil || got != "Қазақстан Республикасы" {
		t.Fatalf("got %q err %v", got, err)
	}
	r := fc.reqs[0]
	if r.Model != "chatgpt-web/high" || r.System != transcribePrompt || len(r.Images) != 1 {
		t.Fatalf("req %+v", r)
	}
	if !strings.Contains(r.System, "ә, ғ, қ, ң, ө, ұ, ү, һ, і") {
		t.Fatal("промпт не содержит казахские буквы")
	}
	if !strings.Contains(r.Text, "Страница 2") || strings.Contains(r.Text, "a.png") {
		t.Fatalf("текст запроса %q: нужен номер страницы и не нужно имя файла", r.Text)
	}
}

func TestPageIncludesTextLayerHint(t *testing.T) {
	fc := &fakeCore{reply: "ok"}
	p := imgPage
	p.TextLayer = "слой"
	if _, err := (&Transcriber{LLM: fc, Model: "m"}).Page(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(fc.reqs[0].Text, "слой") {
		t.Fatalf("нет текстового слоя: %q", fc.reqs[0].Text)
	}
}

func TestPageStripsCodeFence(t *testing.T) {
	fc := &fakeCore{reply: "```markdown\n# Анықтама\nТекст\n```"}
	got, err := (&Transcriber{LLM: fc, Model: "m"}).Page(context.Background(), imgPage)
	if err != nil || got != "# Анықтама\nТекст" {
		t.Fatalf("got %q err %v", got, err)
	}
}

func TestPageEmptyAfterCleanupIsError(t *testing.T) {
	fc := &fakeCore{reply: "```\n```"}
	_, err := (&Transcriber{LLM: fc, Model: "m"}).Page(context.Background(), imgPage)
	if !errors.Is(err, ErrEmpty) {
		t.Fatalf("err %v", err)
	}
}

func TestPageNormalizesToNFC(t *testing.T) {
	fc := &fakeCore{reply: "й"} // «й» в разложенной форме
	got, err := (&Transcriber{LLM: fc, Model: "m"}).Page(context.Background(), imgPage)
	if err != nil || got != "й" {
		t.Fatalf("got %q err %v", got, err)
	}
}

func TestPageTextOnlySkipsCore(t *testing.T) {
	fc := &fakeCore{}
	got, err := (&Transcriber{LLM: fc, Model: "m"}).Page(context.Background(), normalize.Page{Source: "a.docx", Index: 1, TextLayer: "  мәтін \n"})
	if err != nil || got != "мәтін" || len(fc.reqs) != 0 {
		t.Fatalf("got %q err %v reqs %d", got, err, len(fc.reqs))
	}
}

func TestPagePropagatesCoreError(t *testing.T) {
	fc := &fakeCore{err: llm.ErrTurnFailed}
	_, err := (&Transcriber{LLM: fc, Model: "m"}).Page(context.Background(), imgPage)
	if !errors.Is(err, llm.ErrTurnFailed) {
		t.Fatalf("err %v", err)
	}
}

func TestPromptHandlesBlankPage(t *testing.T) {
	if !strings.Contains(transcribePrompt, "[пустая страница]") {
		t.Fatal("промпт должен говорить, что отвечать на пустой странице")
	}
}

func TestPageFixesHomoglyphsInsideCyrillicWords(t *testing.T) {
	// Греческая ο и латинские a, i внутри кириллических слов; чисто латинские слова не трогаем.
	fc := &fakeCore{reply: "Абдыхалыкοву Нұрсұлтaн Өмiрұлы, email: info@umit.kz, ТОО Umit"}
	got, err := (&Transcriber{LLM: fc, Model: "m"}).Page(context.Background(), imgPage)
	want := "Абдыхалыкову Нұрсұлтан Өмірұлы, email: info@umit.kz, ТОО Umit"
	if err != nil || got != want {
		t.Fatalf("got %q, want %q (err %v)", got, want, err)
	}
}
