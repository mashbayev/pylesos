// Файл: internal/telegram/service/login_test.go
package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"pylesos/internal/telegram/domain"
	"pylesos/internal/texts"
)

func newLogin(t *testing.T, gw *fakeGateway, withKeys bool) *Login {
	l, _ := newLoginWith(t, gw, withKeys)
	return l
}

func newLoginWith(t *testing.T, gw *fakeGateway, withKeys bool) (*Login, *fakeListener) {
	t.Helper()
	keys := NewKeyStore(fakeSecrets{})
	if withKeys {
		if err := keys.Save(context.Background(), Keys{AppID: 1, AppHash: "0123456789abcdef0123456789abcdef"}); err != nil {
			t.Fatal(err)
		}
	}
	listener := &fakeListener{}
	return NewLogin(gw, keys, listener), listener
}

func state(t *testing.T, l *Login) LoginState {
	t.Helper()
	st, err := l.State(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func submit(t *testing.T, l *Login, values map[string]string) map[string]string {
	t.Helper()
	problems, err := l.Submit(context.Background(), values)
	if err != nil {
		t.Fatal(err)
	}
	return problems
}

func TestKeysStepValidatesAndSaves(t *testing.T) {
	gw := &fakeGateway{}
	l := newLogin(t, gw, false)
	if st := state(t, l); st.Step != StepKeys {
		t.Fatalf("шаг %q", st.Step)
	}
	problems := submit(t, l, map[string]string{FieldAPIID: "abc", FieldAPIHash: "короткий"})
	if problems[FieldAPIID] != texts.ErrTgAPIID || problems[FieldAPIHash] != texts.ErrTgAPIHash {
		t.Fatalf("ошибки: %v", problems)
	}
	problems = submit(t, l, map[string]string{FieldAPIID: " 1234567 ", FieldAPIHash: "0123456789ABCDEF0123456789abcdef"})
	if len(problems) != 0 || gw.resets != 1 {
		t.Fatalf("ошибки %v, перезапусков %d", problems, gw.resets)
	}
	if st := state(t, l); st.Step != StepPhone {
		t.Fatalf("после ключей шаг %q", st.Step)
	}
}

func TestPhoneCodePasswordFlow(t *testing.T) {
	gw := &fakeGateway{needPassword: true, name: "Тестовый Пользователь"}
	l := newLogin(t, gw, true)
	if p := submit(t, l, map[string]string{FieldPhone: "8 (701) 123-45-67"}); len(p) != 0 || gw.sentPhone != "+77011234567" {
		t.Fatalf("ошибки %v, номер %q", p, gw.sentPhone)
	}
	if st := state(t, l); st.Step != StepCode || st.Phone != "+77011234567" {
		t.Fatalf("шаг кода: %+v", st)
	}
	gw.codeErr = domain.ErrBadCode
	if p := submit(t, l, map[string]string{FieldCode: "11111"}); p[FieldCode] != texts.ErrTgBadCode {
		t.Fatalf("неверный код: %v", p)
	}
	gw.codeErr = nil
	if p := submit(t, l, map[string]string{FieldCode: "22222"}); len(p) != 0 {
		t.Fatalf("код: %v", p)
	}
	if st := state(t, l); st.Step != StepPassword {
		t.Fatalf("шаг пароля: %+v", st)
	}
	gw.passErr = domain.ErrBadPassword
	if p := submit(t, l, map[string]string{FieldPassword: "не тот"}); p[FieldPassword] != texts.ErrTgBadPassword {
		t.Fatalf("неверный пароль: %v", p)
	}
	gw.passErr = nil
	if p := submit(t, l, map[string]string{FieldPassword: "верный"}); len(p) != 0 {
		t.Fatalf("пароль: %v", p)
	}
	if st := state(t, l); st.Step != StepReady || st.Name != "Тестовый Пользователь" {
		t.Fatalf("после входа: %+v", st)
	}
}

func TestExpiredCodeReturnsToPhone(t *testing.T) {
	gw := &fakeGateway{}
	l := newLogin(t, gw, true)
	submit(t, l, map[string]string{FieldPhone: "+77011234567"})
	gw.codeErr = domain.ErrCodeExpired
	if p := submit(t, l, map[string]string{FieldCode: "33333"}); p[FieldPhone] != texts.ErrTgCodeExpired {
		t.Fatalf("устаревший код: %v", p)
	}
	if st := state(t, l); st.Step != StepPhone {
		t.Fatalf("после устаревшего кода шаг %q", st.Step)
	}
}

func TestPhoneProblems(t *testing.T) {
	gw := &fakeGateway{}
	l := newLogin(t, gw, true)
	if p := submit(t, l, map[string]string{FieldPhone: "позвоните мне"}); p[FieldPhone] != texts.ErrTgPhone || gw.sentPhone != "" {
		t.Fatalf("мусор вместо номера: %v, отправлено %q", p, gw.sentPhone)
	}
	gw.sendErr = &domain.FloodWaitError{Wait: 30 * time.Second}
	if p := submit(t, l, map[string]string{FieldPhone: "+77011234567"}); p[FieldPhone] != fmt.Sprintf(texts.TgFloodWait, 30) {
		t.Fatalf("FLOOD_WAIT: %v", p)
	}
	gw.sendErr = domain.ErrBadPhone
	if p := submit(t, l, map[string]string{FieldPhone: "+77011234567"}); p[FieldPhone] != texts.ErrTgBadPhone {
		t.Fatalf("номер не принят: %v", p)
	}
}

// Ревью M2, I1: Telegram не принял ключи — возврат к их вводу с ошибкой у поля; ключи можно сменить и потом.
func TestBadKeysReturnToKeysStepAndEditKeys(t *testing.T) {
	gw := &fakeGateway{sendErr: domain.ErrBadKeys}
	l := newLogin(t, gw, true)
	if p := submit(t, l, map[string]string{FieldPhone: "+77011234567"}); p[FieldAPIHash] != texts.ErrTgBadKeys {
		t.Fatalf("неверные ключи: %v", p)
	}
	if st := state(t, l); st.Step != StepKeys {
		t.Fatalf("шаг %q, ждали ключи", st.Step)
	}
	gw.sendErr = nil
	submit(t, l, map[string]string{FieldAPIID: "7654321", FieldAPIHash: "fedcba9876543210fedcba9876543210"})
	if st := state(t, l); st.Step != StepPhone || gw.resets != 1 {
		t.Fatalf("после новых ключей шаг %q, перезапусков %d", st.Step, gw.resets)
	}
	gw.authorized = true
	l.EditKeys()
	if st := state(t, l); st.Step != StepKeys {
		t.Fatalf("«Изменить ключи» при входе: шаг %q", st.Step)
	}
}

// Ревью M2, I2: на номер нет аккаунта — понятная ошибка у номера, а не «внутренняя ошибка».
func TestNoAccountReturnsToPhone(t *testing.T) {
	gw := &fakeGateway{}
	l := newLogin(t, gw, true)
	submit(t, l, map[string]string{FieldPhone: "+77011234567"})
	gw.codeErr = domain.ErrNoAccount
	if p := submit(t, l, map[string]string{FieldCode: "12345"}); p[FieldPhone] != texts.ErrTgNoAccount {
		t.Fatalf("нет аккаунта: %v", p)
	}
	if st := state(t, l); st.Step != StepPhone {
		t.Fatalf("шаг %q", st.Step)
	}
}

// Ревью M2, I5: после входа сбор продолжается сам; без связи — отдельный шаг, а не ошибка экрана.
func TestLoginNotifiesListenerAndOffline(t *testing.T) {
	gw := &fakeGateway{needPassword: true}
	l, listener := newLoginWith(t, gw, true)
	submit(t, l, map[string]string{FieldPhone: "+77011234567"})
	submit(t, l, map[string]string{FieldCode: "12345"})
	if listener.calls != 0 {
		t.Fatal("оповестили до пароля")
	}
	submit(t, l, map[string]string{FieldPassword: "верный"})
	if listener.calls != 1 {
		t.Fatalf("оповещений после входа %d", listener.calls)
	}
	gw.accountErr = domain.ErrUnavailable
	if st := state(t, l); st.Step != StepOffline {
		t.Fatalf("без связи шаг %q", st.Step)
	}
}
