// Файл: internal/telegram/service/fake_test.go
package service

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"

	"pylesos/internal/telegram/domain"
)

// fakeGateway — Telegram в памяти: вход и одна группа с сообщениями.
type fakeGateway struct {
	mu           sync.Mutex
	accountErr   error
	authorized   bool
	name         string
	sentPhone    string
	sendErr      error
	codeErr      error
	passErr      error
	needPassword bool
	resets       int
	groups       []domain.Group
	msgs         map[int]domain.Message
	flood        time.Duration // одна FLOOD_WAIT на ближайший History
	historyErr   error
	minIDs       []int        // minID каждого вызова History
	service      map[int]bool // служебные сообщения («вступил в группу»): занимают место в порции, но не возвращаются
	calls        int          // сколько раз вызван History
	failAt       int          // на каком вызове History оборвать сбор (0 — не обрывать)
	unavailable  int          // сколько раз подряд ответить «нет связи»
}

func (f *fakeGateway) Account(context.Context) (domain.Account, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return domain.Account{Authorized: f.authorized, Name: f.name}, f.accountErr
}

// fakeListener — кого Login оповещает после входа.
type fakeListener struct{ calls int }

func (f *fakeListener) LoggedIn(context.Context) error {
	f.calls++
	return nil
}

func (f *fakeGateway) SendCode(_ context.Context, phone string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sentPhone = phone
	return "hash-1", f.sendErr
}

func (f *fakeGateway) SignIn(_ context.Context, _, _, hash string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch {
	case f.codeErr != nil:
		return f.codeErr
	case hash != "hash-1":
		return domain.ErrBadCode
	case f.needPassword:
		return domain.ErrPasswordNeeded
	}
	f.authorized = true
	return nil
}

func (f *fakeGateway) Password(context.Context, string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.passErr != nil {
		return f.passErr
	}
	f.authorized = true
	return nil
}

func (f *fakeGateway) Groups(context.Context) ([]domain.Group, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]domain.Group{}, f.groups...), nil
}

func (f *fakeGateway) History(_ context.Context, _ domain.Peer, offsetID, minID, limit int) (domain.Page, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	switch {
	case f.flood > 0:
		wait := f.flood
		f.flood = 0
		return domain.Page{}, &domain.FloodWaitError{Wait: wait}
	case f.unavailable > 0:
		f.unavailable--
		return domain.Page{}, domain.ErrUnavailable
	case f.failAt == f.calls:
		return domain.Page{}, errors.New("обрыв соединения")
	case f.historyErr != nil:
		return domain.Page{}, f.historyErr
	}
	f.minIDs = append(f.minIDs, minID)
	return f.page(offsetID, minID, limit), nil
}

// page — порция как у Telegram: служебные сообщения занимают место, но в Messages не попадают.
func (f *fakeGateway) page(offsetID, minID, limit int) domain.Page {
	ids := []int{}
	for id := range f.msgs {
		if (offsetID == 0 || id < offsetID) && id > minID {
			ids = append(ids, id)
		}
	}
	for id := range f.service {
		if (offsetID == 0 || id < offsetID) && id > minID {
			ids = append(ids, id)
		}
	}
	sort.Sort(sort.Reverse(sort.IntSlice(ids)))
	if len(ids) > limit {
		ids = ids[:limit]
	}
	p := domain.Page{Messages: []domain.Message{}}
	if len(ids) == 0 {
		return p
	}
	p.Highest, p.Lowest = ids[0], ids[len(ids)-1]
	for _, id := range ids {
		if m, ok := f.msgs[id]; ok {
			p.Messages = append(p.Messages, m)
		}
	}
	return p
}

func (f *fakeGateway) Alive(_ context.Context, _ domain.Peer, ids []int) (map[int]bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	alive := map[int]bool{}
	for _, id := range ids {
		if _, ok := f.msgs[id]; ok {
			alive[id] = true
		}
	}
	return alive, nil
}

func (f *fakeGateway) Reset() {
	f.mu.Lock()
	f.resets++
	f.mu.Unlock()
}

// fakeSecrets — секреты в памяти.
type fakeSecrets map[string][]byte

func (s fakeSecrets) Get(_ context.Context, key string) ([]byte, bool, error) {
	v, ok := s[key]
	return v, ok, nil
}

func (s fakeSecrets) Put(_ context.Context, key string, value []byte) error {
	s[key] = value
	return nil
}
