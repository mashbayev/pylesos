// Файл: internal/telegram/service/groups.go
package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	jobsdomain "pylesos/internal/jobs/domain"
	sourcesdomain "pylesos/internal/sources/domain"
	"pylesos/internal/telegram/domain"
)

// KindSync — вид задания «сбор Telegram».
const KindSync = "tg_sync"

type SourceStore interface {
	Ensure(ctx context.Context, s sourcesdomain.Source) (int64, bool, error)
	ListByKind(ctx context.Context, kind sourcesdomain.Kind) ([]sourcesdomain.Source, error)
}

type JobStarter interface {
	Start(ctx context.Context, sourceID int64, kind, params string) (int64, error)
	ResumeWaiting(ctx context.Context, kind string) error
}

var ErrGroupGone = errors.New("группа не найдена в списке")

type GroupView struct {
	Group domain.Group
	Added bool // уже источник Пылесоса
}

type Groups struct {
	gw      Gateway
	sources SourceStore
	jobs    JobStarter
	now     func() time.Time

	mu    sync.Mutex
	cache []domain.Group
}

func NewGroups(gw Gateway, sources SourceStore, jobs JobStarter, now func() time.Time) *Groups {
	return &Groups{gw: gw, sources: sources, jobs: jobs, now: now}
}

// List — группы аккаунта с отметкой «добавлена». Список из Telegram кэшируется; refresh — перечитать.
func (g *Groups) List(ctx context.Context, refresh bool) ([]GroupView, error) {
	groups, err := g.groups(ctx, refresh)
	if err != nil {
		return nil, err
	}
	added, err := g.addedConfigs(ctx)
	if err != nil {
		return nil, err
	}
	views := make([]GroupView, 0, len(groups))
	for _, gr := range groups {
		cfg, err := configOf(gr.Peer)
		if err != nil {
			return nil, err
		}
		views = append(views, GroupView{Group: gr, Added: added[cfg]})
	}
	return views, nil
}

// Add делает группу источником и сразу запускает сбор; идущий сбор — не ошибка.
func (g *Groups) Add(ctx context.Context, key string) error {
	peer, err := domain.ParsePeerKey(key)
	if err != nil {
		return ErrGroupGone
	}
	group, ok := g.find(peer)
	if !ok {
		return ErrGroupGone
	}
	cfg, err := configOf(peer)
	if err != nil {
		return err
	}
	id, _, err := g.sources.Ensure(ctx, sourcesdomain.Source{Kind: sourcesdomain.KindTelegramGroup,
		Title: group.Title, Config: cfg, CreatedAt: g.now().Unix()})
	if err != nil {
		return err
	}
	return ignoreBusy(g.Collect(ctx, id))
}

// Collect — кнопка «Собрать»: занятый источник — ошибка ErrSourceBusy для экрана.
func (g *Groups) Collect(ctx context.Context, sourceID int64) error {
	_, err := g.jobs.Start(ctx, sourceID, KindSync, "{}")
	return err
}

// LoggedIn — вход выполнен: продолжить и догрузить сбор (LoginListener).
func (g *Groups) LoggedIn(ctx context.Context) error { return g.SyncAll(ctx) }

// SyncAll при запуске программы и после входа догружает все группы, если вход выполнен
// (спецификация, раздел 3, п. 4), и продолжает задания, ждавшие входа.
func (g *Groups) SyncAll(ctx context.Context) error {
	acc, err := g.gw.Account(ctx)
	if errors.Is(err, domain.ErrNotConfigured) {
		return nil // ключи ещё не введены — догружать нечего
	}
	if err != nil || !acc.Authorized {
		return err
	}
	if err := g.jobs.ResumeWaiting(ctx, KindSync); err != nil {
		return err
	}
	list, err := g.sources.ListByKind(ctx, sourcesdomain.KindTelegramGroup)
	if err != nil {
		return err
	}
	for _, s := range list {
		if err := ignoreBusy(g.Collect(ctx, s.ID)); err != nil {
			return err
		}
	}
	return nil
}

func (g *Groups) groups(ctx context.Context, refresh bool) ([]domain.Group, error) {
	g.mu.Lock()
	cached := g.cache
	g.mu.Unlock()
	if cached != nil && !refresh {
		return cached, nil
	}
	fresh, err := g.gw.Groups(ctx)
	if err != nil {
		return nil, err
	}
	g.mu.Lock()
	g.cache = fresh
	g.mu.Unlock()
	return fresh, nil
}

func (g *Groups) find(p domain.Peer) (domain.Group, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	for _, gr := range g.cache {
		if gr.Peer == p {
			return gr, true
		}
	}
	return domain.Group{}, false
}

func (g *Groups) addedConfigs(ctx context.Context) (map[string]bool, error) {
	list, err := g.sources.ListByKind(ctx, sourcesdomain.KindTelegramGroup)
	if err != nil {
		return nil, err
	}
	added := make(map[string]bool, len(list))
	for _, s := range list {
		added[s.Config] = true
	}
	return added, nil
}

// configOf — JSON группы для sources.config (порядок полей постоянный — годится для сравнения).
func configOf(p domain.Peer) (string, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return "", fmt.Errorf("группа в JSON: %w", err)
	}
	return string(data), nil
}

func ignoreBusy(err error) error {
	if errors.Is(err, jobsdomain.ErrSourceBusy) {
		return nil
	}
	return err
}
