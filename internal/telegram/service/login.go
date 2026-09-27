// Файл: internal/telegram/service/login.go
package service

import (
	"context"
	"errors"
	"strings"
	"sync"

	"pylesos/internal/telegram/domain"
	"pylesos/internal/texts"
)

type Step string

const (
	StepKeys     Step = "keys"
	StepPhone    Step = "phone"
	StepCode     Step = "code"
	StepPassword Step = "password"
	StepReady    Step = "ready"
	StepOffline  Step = "offline" // нет связи с Telegram: экран предлагает повторить или сменить ключи
)

// LoginListener — кого оповестить после входа (продолжить сбор, ждущий «войдите в Telegram»).
type LoginListener interface {
	LoggedIn(ctx context.Context) error
}

type LoginState struct {
	Step  Step
	Name  string // имя аккаунта, когда вход выполнен
	Phone string // номер, на который отправлен код
}

// Login ведёт вход по шагам: ключи API → номер → код → облачный пароль.
type Login struct {
	gw       Gateway
	keys     *KeyStore
	listener LoginListener

	mu    sync.Mutex
	step  Step // StepKeys, StepCode или StepPassword, пока идёт вход или смена ключей; иначе ""
	phone string
	hash  string
}

func NewLogin(gw Gateway, keys *KeyStore, listener LoginListener) *Login {
	return &Login{gw: gw, keys: keys, listener: listener}
}

// EditKeys — «Изменить api_id и api_hash»: следующий шаг — ввод ключей.
func (l *Login) EditKeys() { l.set(StepKeys, "", "") }

func (l *Login) State(ctx context.Context) (LoginState, error) {
	if _, ok, err := l.keys.Load(ctx); err != nil || !ok {
		return LoginState{Step: StepKeys}, err
	}
	l.mu.Lock()
	pending := LoginState{Step: l.step, Phone: l.phone}
	l.mu.Unlock()
	if pending.Step != "" {
		return pending, nil
	}
	acc, err := l.gw.Account(ctx)
	switch {
	case errors.Is(err, domain.ErrUnavailable):
		return LoginState{Step: StepOffline}, nil
	case errors.Is(err, domain.ErrBadKeys):
		return LoginState{Step: StepKeys}, nil
	case err != nil:
		return LoginState{}, err
	}
	if acc.Authorized {
		return LoginState{Step: StepReady, Name: acc.Name}, nil
	}
	return LoginState{Step: StepPhone}, nil
}

// Submit выполняет текущий шаг; ошибки ввода — по полям, готовым текстом.
func (l *Login) Submit(ctx context.Context, values map[string]string) (map[string]string, error) {
	st, err := l.State(ctx)
	if err != nil {
		return nil, err
	}
	switch st.Step {
	case StepKeys:
		return l.submitKeys(ctx, values)
	case StepPhone:
		return l.submitPhone(ctx, values[FieldPhone])
	case StepCode:
		return l.submitCode(ctx, st.Phone, values[FieldCode])
	case StepPassword:
		return l.submitPassword(ctx, values[FieldPassword])
	default:
		return map[string]string{}, nil
	}
}

func (l *Login) submitKeys(ctx context.Context, values map[string]string) (map[string]string, error) {
	keys, problems := validateKeys(values)
	if len(problems) > 0 {
		return problems, nil
	}
	if err := l.keys.Save(ctx, keys); err != nil {
		return nil, err
	}
	l.set("", "", "")
	l.gw.Reset()
	return map[string]string{}, nil
}

func (l *Login) submitPhone(ctx context.Context, raw string) (map[string]string, error) {
	phone, ok := normalizePhone(raw)
	if !ok {
		return map[string]string{FieldPhone: texts.ErrTgPhone}, nil
	}
	hash, err := l.gw.SendCode(ctx, phone)
	if errors.Is(err, domain.ErrBadKeys) {
		l.set(StepKeys, "", "")
		return map[string]string{FieldAPIHash: texts.ErrTgBadKeys}, nil
	}
	if p := problemOf(err); p != "" {
		return map[string]string{FieldPhone: p}, nil
	}
	if err != nil {
		return nil, err
	}
	l.set(StepCode, phone, hash)
	return map[string]string{}, nil
}

func (l *Login) submitCode(ctx context.Context, phone, raw string) (map[string]string, error) {
	code := strings.TrimSpace(raw)
	if code == "" {
		return map[string]string{FieldCode: texts.ErrTgBadCode}, nil
	}
	l.mu.Lock()
	hash := l.hash
	l.mu.Unlock()
	err := l.gw.SignIn(ctx, phone, code, hash)
	switch {
	case errors.Is(err, domain.ErrPasswordNeeded):
		l.set(StepPassword, phone, hash)
		return map[string]string{}, nil
	case errors.Is(err, domain.ErrCodeExpired):
		l.set("", "", "")
		return map[string]string{FieldPhone: texts.ErrTgCodeExpired}, nil
	case errors.Is(err, domain.ErrNoAccount):
		l.set("", "", "")
		return map[string]string{FieldPhone: texts.ErrTgNoAccount}, nil
	}
	if p := problemOf(err); p != "" {
		return map[string]string{FieldCode: p}, nil
	}
	if err != nil {
		return nil, err
	}
	return l.loggedIn(ctx)
}

func (l *Login) submitPassword(ctx context.Context, password string) (map[string]string, error) {
	err := l.gw.Password(ctx, password)
	if p := problemOf(err); p != "" {
		return map[string]string{FieldPassword: p}, nil
	}
	if err != nil {
		return nil, err
	}
	return l.loggedIn(ctx)
}

// loggedIn — вход выполнен: сбросить шаги и продолжить сбор, ждавший входа (ревью M2, I5).
func (l *Login) loggedIn(ctx context.Context) (map[string]string, error) {
	l.set("", "", "")
	if err := l.listener.LoggedIn(ctx); err != nil {
		return nil, err
	}
	return map[string]string{}, nil
}

func (l *Login) set(step Step, phone, hash string) {
	l.mu.Lock()
	l.step, l.phone, l.hash = step, phone, hash
	l.mu.Unlock()
}
