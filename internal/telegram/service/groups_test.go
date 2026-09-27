// Файл: internal/telegram/service/groups_test.go
package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	jobsdomain "pylesos/internal/jobs/domain"
	sourcesdomain "pylesos/internal/sources/domain"
	sourcesrepo "pylesos/internal/sources/repository"
	"pylesos/internal/store/storetest"
	"pylesos/internal/telegram/domain"
)

type fakeStarter struct {
	mu      sync.Mutex
	started []int64
	resumed []string
	err     error
}

func (f *fakeStarter) ResumeWaiting(_ context.Context, kind string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.resumed = append(f.resumed, kind)
	return nil
}

func (f *fakeStarter) Start(_ context.Context, sourceID int64, kind, _ string) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if kind != KindSync {
		return 0, errors.New("не тот вид задания: " + kind)
	}
	f.started = append(f.started, sourceID)
	return int64(len(f.started)), f.err
}

var testGroup = domain.Group{Peer: domain.Peer{Type: domain.PeerChannel, ID: 10, AccessHash: 20}, Title: "Тестовая группа"}

func newGroups(t *testing.T, gw *fakeGateway) (*Groups, *sourcesrepo.SQLite, *fakeStarter) {
	t.Helper()
	repo := sourcesrepo.NewSQLite(storetest.Open(t))
	starter := &fakeStarter{}
	now := func() time.Time { return time.Unix(1000, 0) }
	return NewGroups(gw, repo, starter, now), repo, starter
}

func TestAddGroupCreatesSourceOnceAndStartsSync(t *testing.T) {
	ctx := context.Background()
	g, repo, starter := newGroups(t, &fakeGateway{authorized: true, groups: []domain.Group{testGroup}})
	views, err := g.List(ctx, false)
	if err != nil || len(views) != 1 || views[0].Added {
		t.Fatalf("список: %+v %v", views, err)
	}
	if err := g.Add(ctx, testGroup.Peer.Key()); err != nil {
		t.Fatal(err)
	}
	starter.err = jobsdomain.ErrSourceBusy // второй клик, пока идёт сбор
	if err := g.Add(ctx, testGroup.Peer.Key()); err != nil {
		t.Fatalf("повторное добавление: %v", err)
	}
	list, err := repo.ListByKind(ctx, sourcesdomain.KindTelegramGroup)
	if err != nil || len(list) != 1 || list[0].Title != "Тестовая группа" {
		t.Fatalf("источники: %+v %v", list, err)
	}
	if len(starter.started) != 2 || starter.started[0] != list[0].ID {
		t.Fatalf("запуски сбора: %v", starter.started)
	}
	views, err = g.List(ctx, false)
	if err != nil || !views[0].Added {
		t.Fatalf("после добавления: %+v %v", views, err)
	}
	if err := g.Collect(ctx, list[0].ID); !errors.Is(err, jobsdomain.ErrSourceBusy) {
		t.Fatalf("«Собрать» при идущем сборе: %v", err)
	}
}

func TestAddUnknownGroup(t *testing.T) {
	g, _, _ := newGroups(t, &fakeGateway{authorized: true})
	for _, key := range []string{"мусор", testGroup.Peer.Key()} {
		if err := g.Add(context.Background(), key); !errors.Is(err, ErrGroupGone) {
			t.Fatalf("%q: %v", key, err)
		}
	}
}

func TestSyncAllOnlyWhenAuthorized(t *testing.T) {
	ctx := context.Background()
	gw := &fakeGateway{authorized: true, groups: []domain.Group{testGroup}}
	g, _, starter := newGroups(t, gw)
	if _, err := g.List(ctx, false); err != nil {
		t.Fatal(err)
	}
	if err := g.Add(ctx, testGroup.Peer.Key()); err != nil {
		t.Fatal(err)
	}
	gw.authorized = false
	if err := g.SyncAll(ctx); err != nil || len(starter.started) != 1 {
		t.Fatalf("без входа: %v, запусков %d", err, len(starter.started))
	}
	gw.authorized = true
	starter.err = jobsdomain.ErrSourceBusy // задание уже продолжено после перезапуска
	if err := g.SyncAll(ctx); err != nil || len(starter.started) != 2 {
		t.Fatalf("с входом: %v, запусков %d", err, len(starter.started))
	}
	if len(starter.resumed) != 1 || starter.resumed[0] != KindSync {
		t.Fatalf("ждущие «войдите в Telegram» не продолжены: %v", starter.resumed)
	}
	if err := g.LoggedIn(ctx); err != nil || len(starter.resumed) != 2 {
		t.Fatalf("после входа: %v, продолжений %v", err, starter.resumed)
	}
}
