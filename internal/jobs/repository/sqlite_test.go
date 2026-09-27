// Файл: internal/jobs/repository/sqlite_test.go
package repository

import (
	"context"
	"errors"
	"testing"

	"pylesos/internal/jobs/domain"
	"pylesos/internal/store/storetest"
)

func newJob(sourceID int64, status domain.Status) domain.Job {
	return domain.Job{SourceID: sourceID, Kind: "test", Params: "{}", Status: status, Cursor: "{}", CreatedAt: 100, UpdatedAt: 100}
}

func TestCreateGetAndBusySource(t *testing.T) {
	ctx := context.Background()
	db := storetest.Open(t)
	src := storetest.AddSource(t, db, "Группа А")
	repo := NewSQLite(db)
	id, err := repo.Create(ctx, newJob(src, domain.StatusQueued))
	if err != nil {
		t.Fatal(err)
	}
	job, err := repo.Get(ctx, id)
	if err != nil || job.Status != domain.StatusQueued || job.Kind != "test" {
		t.Fatalf("job=%+v err=%v", job, err)
	}
	if _, err := repo.Create(ctx, newJob(src, domain.StatusQueued)); !errors.Is(err, domain.ErrSourceBusy) {
		t.Fatalf("второе активное задание: %v", err)
	}
	if err := repo.UpdateState(ctx, id, domain.StateUpdate{Status: domain.StatusDone, At: 200, Finished: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Create(ctx, newJob(src, domain.StatusQueued)); err != nil {
		t.Fatalf("новое задание после завершённого: %v", err)
	}
	if err := repo.UpdateState(ctx, id, domain.StateUpdate{Status: domain.StatusRunning, At: 300}); !errors.Is(err, domain.ErrSourceBusy) {
		t.Fatalf("возобновление при другом активном: %v", err)
	}
}

func TestGetAndUpdateMissing(t *testing.T) {
	ctx := context.Background()
	repo := NewSQLite(storetest.Open(t))
	if _, err := repo.Get(ctx, 42); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("Get: %v", err)
	}
	if err := repo.UpdateState(ctx, 42, domain.StateUpdate{Status: domain.StatusDone}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("UpdateState: %v", err)
	}
	if err := repo.SaveCursor(ctx, 42, "{}", "", 0); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("SaveCursor: %v", err)
	}
}

func TestListAndListByStatus(t *testing.T) {
	ctx := context.Background()
	db := storetest.Open(t)
	a := storetest.AddSource(t, db, "Группа А")
	b := storetest.AddSource(t, db, "Поиск Б")
	repo := NewSQLite(db)
	first, err := repo.Create(ctx, newJob(a, domain.StatusRunning))
	if err != nil {
		t.Fatal(err)
	}
	second, err := repo.Create(ctx, newJob(b, domain.StatusPaused))
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveCursor(ctx, first, `{"page":3}`, "страница 3", 150); err != nil {
		t.Fatal(err)
	}
	items, err := repo.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].ID != second || items[1].SourceTitle != "Группа А" || items[1].Cursor != `{"page":3}` {
		t.Fatalf("List: %+v", items)
	}
	running, err := repo.ListByStatus(ctx, domain.StatusQueued, domain.StatusRunning)
	if err != nil || len(running) != 1 || running[0].ID != first {
		t.Fatalf("ListByStatus: %+v %v", running, err)
	}
	empty, err := NewSQLite(storetest.Open(t)).List(ctx)
	if err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("пустой список должен быть [], не nil: %#v %v", empty, err)
	}
}

// Условная смена статуса: переход только из перечисленных статусов (гонки Resume/Cancel).
func TestUpdateStateFromGuard(t *testing.T) {
	ctx := context.Background()
	db := storetest.Open(t)
	repo := NewSQLite(db)
	id, err := repo.Create(ctx, newJob(storetest.AddSource(t, db, "Группа"), domain.StatusPaused))
	if err != nil {
		t.Fatal(err)
	}
	cancel := domain.StateUpdate{Status: domain.StatusCancelled, At: 200, Finished: true}
	if err := repo.UpdateState(ctx, id, cancel); err != nil {
		t.Fatal(err)
	}
	run := domain.StateUpdate{Status: domain.StatusRunning, At: 300,
		From: []domain.Status{domain.StatusPaused, domain.StatusWaitingUser, domain.StatusFailed}}
	if err := repo.UpdateState(ctx, id, run); !errors.Is(err, domain.ErrStateChanged) {
		t.Fatalf("переход из «отменено» в «идёт»: %v", err)
	}
	if job, err := repo.Get(ctx, id); err != nil || job.Status != domain.StatusCancelled {
		t.Fatalf("статус перезаписан: %+v %v", job, err)
	}
	if err := repo.UpdateState(ctx, 999, run); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("нет задания: %v", err)
	}
}

func TestSourceStats(t *testing.T) {
	ctx := context.Background()
	db := storetest.Open(t)
	a := storetest.AddSource(t, db, "А")
	b := storetest.AddSource(t, db, "Б")
	repo := NewSQLite(db)
	done, err := repo.Create(ctx, newJob(a, domain.StatusQueued))
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.UpdateState(ctx, done, domain.StateUpdate{Status: domain.StatusDone, At: 700, Finished: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Create(ctx, newJob(b, domain.StatusRunning)); err != nil {
		t.Fatal(err)
	}
	stats, err := repo.SourceStats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats[a].Active || stats[a].LastDone != 700 || !stats[b].Active || stats[b].LastDone != 0 {
		t.Fatalf("статистика: %+v", stats)
	}
}
