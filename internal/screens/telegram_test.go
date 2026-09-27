// Файл: internal/screens/telegram_test.go
package screens

import (
	"context"
	"fmt"
	"testing"
	"time"

	tgdomain "pylesos/internal/telegram/domain"
	tgservice "pylesos/internal/telegram/service"
	"pylesos/internal/texts"
)

type fakeLogin struct {
	state    tgservice.LoginState
	problems map[string]string
	got      map[string]string
	edited   bool
}

func (f *fakeLogin) EditKeys() {
	f.edited = true
	f.state = tgservice.LoginState{Step: tgservice.StepKeys}
}

func (f *fakeLogin) State(context.Context) (tgservice.LoginState, error) { return f.state, nil }
func (f *fakeLogin) Submit(_ context.Context, v map[string]string) (map[string]string, error) {
	f.got = v
	return f.problems, nil
}

type fakeGroups struct {
	views   []tgservice.GroupView
	addErr  error
	added   string
	refresh bool
}

func (f *fakeGroups) List(_ context.Context, refresh bool) ([]tgservice.GroupView, error) {
	f.refresh = refresh
	return f.views, nil
}
func (f *fakeGroups) Add(_ context.Context, key string) error { f.added = key; return f.addErr }

func tgBuilder(l *fakeLogin, g *fakeGroups) *Builder {
	return NewBuilder(Deps{Jobs: fakeJobs{}, Kinds: fakeKinds{}, Settings: &fakeSettings{}, Sources: fakeSources{},
		Counts: fakeCounts{}, Stats: fakeStats{}, Login: l, Groups: g, Bot: &fakeBot{}, Forms: &fakeForms{}, Ollama: fakeOllama{}, Loc: time.UTC})
}

func TestTelegramLoginStepKeepsInputExceptPassword(t *testing.T) {
	l := &fakeLogin{state: tgservice.LoginState{Step: tgservice.StepKeys},
		problems: map[string]string{tgservice.FieldAPIHash: texts.ErrTgAPIHash}}
	s, err := tgBuilder(l, &fakeGroups{}).TelegramSubmit(context.Background(),
		map[string]string{tgservice.FieldAPIID: "123", tgservice.FieldAPIHash: "плохой"})
	if err != nil {
		t.Fatal(err)
	}
	if s.Hint != texts.TgKeysHint || len(s.Fields) != 2 || s.Fields[0].Value != "123" || s.Fields[1].Error != texts.ErrTgAPIHash {
		t.Fatalf("шаг ключей: %+v", s)
	}
	l.state = tgservice.LoginState{Step: tgservice.StepPassword}
	l.problems = map[string]string{tgservice.FieldPassword: texts.ErrTgBadPassword}
	s, err = tgBuilder(l, &fakeGroups{}).TelegramSubmit(context.Background(), map[string]string{tgservice.FieldPassword: "секрет"})
	if err != nil {
		t.Fatal(err)
	}
	if s.Fields[0].Kind != "password" || s.Fields[0].Value != "" || s.Fields[0].Error != texts.ErrTgBadPassword {
		t.Fatalf("пароль вернулся в форму или нет ошибки: %+v", s.Fields[0])
	}
	l.state = tgservice.LoginState{Step: tgservice.StepCode, Phone: "+77011234567"}
	s, err = tgBuilder(l, &fakeGroups{}).Telegram(context.Background())
	if err != nil || s.Hint != fmt.Sprintf(texts.TgCodeHint, "+77011234567") {
		t.Fatalf("подсказка кода: %q %v", s.Hint, err)
	}
}

func TestTelegramReadyListsGroups(t *testing.T) {
	g := &fakeGroups{views: []tgservice.GroupView{
		{Group: tgdomain.Group{Peer: tgdomain.Peer{Type: tgdomain.PeerChannel, ID: 1, AccessHash: 2}, Title: "Новая"}},
		{Group: tgdomain.Group{Peer: tgdomain.Peer{Type: tgdomain.PeerChat, ID: 3}, Title: "Добавленная"}, Added: true},
	}}
	b := tgBuilder(&fakeLogin{state: tgservice.LoginState{Step: tgservice.StepReady, Name: "Тест"}}, g)
	s, err := b.TelegramRefresh(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !g.refresh || s.Status != fmt.Sprintf(texts.TgLoggedIn, "Тест") || len(s.Fields) != 0 {
		t.Fatalf("экран: %+v", s)
	}
	if s.Groups[0].Key != "channel:1:2" || s.Groups[0].Action == nil || s.Groups[1].Action != nil || s.Groups[1].Status != texts.TgAdded {
		t.Fatalf("группы: %+v", s.Groups)
	}
	g.addErr = tgservice.ErrGroupGone
	if s, err := b.TelegramAddGroup(context.Background(), "channel:1:2"); err != nil || s.Message != texts.ErrTgGroupGone || g.added != "channel:1:2" {
		t.Fatalf("исчезнувшая группа: %+v %v", s, err)
	}
	g.addErr = nil
	if s, err := b.TelegramAddGroup(context.Background(), "channel:1:2"); err != nil || s.Message != texts.TgGroupAdded {
		t.Fatalf("добавление: %+v %v", s, err)
	}
}

func TestHomeTelegramRows(t *testing.T) {
	b := NewBuilder(Deps{Jobs: fakeJobs{}, Kinds: fakeKinds{}, Settings: &fakeSettings{},
		Sources: fakeSources{{ID: 1, Title: "Группа А"}, {ID: 2, Title: "Группа Б"}},
		Counts:  fakeCounts{1: 124},
		Stats:   fakeStats{1: {SourceID: 1, LastDone: time.Date(2026, 9, 27, 9, 30, 0, 0, time.UTC).Unix()}, 2: {SourceID: 2, Active: true}},
		Login:   &fakeLogin{}, Groups: &fakeGroups{}, Bot: &fakeBot{}, Forms: &fakeForms{}, Ollama: fakeOllama{}, Loc: time.UTC})
	s, err := b.Home(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if s.EmptyText != "" || s.TelegramTitle != texts.HomeTelegramTitle || len(s.Telegram) != 2 {
		t.Fatalf("главная: %+v", s)
	}
	a := s.Telegram[0]
	if a.Lines[0] != fmt.Sprintf(texts.TgMessagesCount, 124) || a.Lines[1] != fmt.Sprintf(texts.TgLastSync, "27.09.2026 09:30") ||
		len(a.Actions) != 3 || a.Actions[0].ID != "collect" || a.Actions[1].ID != "analyze" || a.Actions[2].ID != "export" || a.Actions[1].Label != texts.AnalyzeCollect {
		t.Fatalf("группа А: %+v", a)
	}
	if bb := s.Telegram[1]; bb.Lines[1] != texts.TgNeverSynced || len(bb.Actions) != 0 {
		t.Fatalf("группа Б (идёт сбор): %+v", bb)
	}
}

// Ревью M2, I1/I3: без связи — понятное сообщение и выход «сменить ключи»; ключи можно сменить и после входа.
func TestTelegramOfflineAndEditKeys(t *testing.T) {
	l := &fakeLogin{state: tgservice.LoginState{Step: tgservice.StepOffline}}
	b := tgBuilder(l, &fakeGroups{})
	s, err := b.Telegram(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if s.Message != texts.TgNoConnection || s.EditKeysLabel != texts.TgEditKeys || len(s.Fields) != 0 || len(s.Groups) != 0 {
		t.Fatalf("без связи: %+v", s)
	}
	l.state = tgservice.LoginState{Step: tgservice.StepReady, Name: "Тест"}
	if s, err := b.Telegram(context.Background()); err != nil || s.EditKeysLabel != texts.TgEditKeys {
		t.Fatalf("после входа нет «Изменить ключи»: %+v %v", s, err)
	}
	s, err = b.TelegramEditKeys(context.Background())
	if err != nil || !l.edited || s.Hint != texts.TgKeysHint || s.EditKeysLabel != "" {
		t.Fatalf("смена ключей: %+v %v", s, err)
	}
}

type fakeBot struct {
	status  tgservice.BotStatus
	problem string
	token   string
}

func (f *fakeBot) Status() tgservice.BotStatus { return f.status }
func (f *fakeBot) SetToken(_ context.Context, token string) (string, error) {
	f.token = token
	return f.problem, nil
}

func TestTelegramBotSection(t *testing.T) {
	bot := &fakeBot{status: tgservice.BotStatus{State: tgservice.BotRunning, Username: "pylesos_bot"}}
	b := NewBuilder(Deps{Jobs: fakeJobs{}, Kinds: fakeKinds{}, Settings: &fakeSettings{}, Sources: fakeSources{},
		Counts: fakeCounts{}, Stats: fakeStats{}, Login: &fakeLogin{state: tgservice.LoginState{Step: tgservice.StepKeys}},
		Groups: &fakeGroups{}, Bot: bot, Forms: &fakeForms{}, Ollama: fakeOllama{}, Loc: time.UTC})
	s, err := b.Telegram(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if s.BotTitle != texts.TgBotTitle || s.BotStatus != fmt.Sprintf(texts.TgBotRunning, "pylesos_bot") ||
		s.BotField.Kind != "password" || s.BotField.Value != "" {
		t.Fatalf("раздел бота при шаге ключей: %+v", s)
	}
	bot.problem = texts.ErrTgBotTokenFormat
	s, err = b.TelegramBot(context.Background(), "мусор")
	if err != nil || bot.token != "мусор" || s.BotField.Error != texts.ErrTgBotTokenFormat {
		t.Fatalf("ошибка токена: %+v %v", s.BotField, err)
	}
	bot.problem, bot.status = "", tgservice.BotStatus{State: tgservice.BotBadToken}
	s, err = b.TelegramBot(context.Background(), "123:abc")
	if err != nil || s.Message != texts.TgBotSaved || s.BotStatus != texts.TgBotBadToken {
		t.Fatalf("после сохранения: %+v %v", s, err)
	}
}
