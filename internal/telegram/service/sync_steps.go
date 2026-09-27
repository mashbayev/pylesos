// Файл: internal/telegram/service/sync_steps.go
package service

import (
	"context"
	"encoding/json"
	"fmt"

	"pylesos/internal/telegram/domain"
	"pylesos/internal/telegram/repository"
	"pylesos/internal/texts"
)

func (r *syncRun) step(ctx context.Context) error {
	switch r.c.Phase {
	case phaseNew:
		return r.stepNew(ctx)
	case phaseOld:
		return r.stepOld(ctx)
	case phaseDeleted:
		return r.checkDeleted(ctx)
	default:
		return fmt.Errorf("неизвестная фаза сбора %q", r.c.Phase)
	}
}

// stepNew — порция сообщений новее отметки источника. Проход сохраняется у источника после каждой порции,
// поэтому оборванный проход продолжит следующее задание; отметка сдвигается только в конце прохода.
func (r *syncRun) stepNew(ctx context.Context) error {
	page, err := r.history(ctx, r.c.Top)
	if err != nil {
		return err
	}
	if page.Lowest == 0 {
		return r.finishNew(ctx)
	}
	if err := r.store(ctx, page.Messages); err != nil {
		return err
	}
	if r.c.Head == 0 {
		r.c.Head = page.Highest
	}
	r.c.Offset = page.Lowest
	st := repository.SyncState{SyncedTop: r.c.Top, PassHead: r.c.Head, PassLow: r.c.Offset}
	if err := r.w.msgs.SaveSyncState(ctx, r.source, st); err != nil {
		return err
	}
	return r.save(ctx, r.progressText())
}

func (r *syncRun) finishNew(ctx context.Context) error {
	top := max(r.c.Top, r.c.Head)
	if err := r.w.msgs.SaveSyncState(ctx, r.source, repository.SyncState{SyncedTop: top}); err != nil {
		return err
	}
	lo, _, err := r.w.msgs.Bounds(ctx, r.source)
	if err != nil {
		return err
	}
	r.c.Phase, r.c.Top, r.c.Head, r.c.Offset = phaseOld, top, 0, lo
	return r.save(ctx, r.progressText())
}

// stepOld — история ниже самого старого сохранённого сообщения, до начала группы.
func (r *syncRun) stepOld(ctx context.Context) error {
	if r.c.Offset == 0 { // в группе нет сообщений
		return r.startDeleted(ctx)
	}
	page, err := r.history(ctx, 0)
	if err != nil {
		return err
	}
	if page.Lowest == 0 {
		return r.startDeleted(ctx)
	}
	if err := r.store(ctx, page.Messages); err != nil {
		return err
	}
	r.c.Offset = page.Lowest
	return r.save(ctx, r.progressText())
}

// startDeleted: удалённые сверяются только в последних window сообщениях (ревью M2, I6).
func (r *syncRun) startDeleted(ctx context.Context) error {
	after, err := r.w.msgs.RecentWindowStart(ctx, r.source, r.w.window)
	if err != nil {
		return err
	}
	r.c.Phase, r.c.Checked = phaseDeleted, after
	return r.save(ctx, r.progressText())
}

// checkDeleted сверяет порцию сохранённых номеров с Telegram и помечает исчезнувшие.
func (r *syncRun) checkDeleted(ctx context.Context) error {
	ids, err := r.w.msgs.LiveIDsAfter(ctx, r.source, r.c.Checked, r.w.pageSize)
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		r.c.Phase = phaseDone
		return r.save(ctx, r.progressText())
	}
	alive, err := retry(ctx, r, func() (map[int]bool, error) { return r.w.gw.Alive(ctx, r.peer, ids) })
	if err != nil {
		return err
	}
	gone := []int{}
	for _, id := range ids {
		if !alive[id] {
			gone = append(gone, id)
		}
	}
	if err := r.w.msgs.MarkDeleted(ctx, r.source, gone); err != nil {
		return err
	}
	r.c.Checked = ids[len(ids)-1]
	r.c.Verified += len(ids)
	return r.save(ctx, r.progressText())
}

func (r *syncRun) store(ctx context.Context, msgs []domain.Message) error {
	if err := r.w.msgs.Save(ctx, r.source, msgs, repository.ViaAccount, r.w.now().Unix()); err != nil {
		return err
	}
	r.c.Loaded += len(msgs)
	return nil
}

func (r *syncRun) save(ctx context.Context, text string) error {
	data, err := json.Marshal(r.c)
	if err != nil {
		return fmt.Errorf("курсор сбора: %w", err)
	}
	return r.p.Save(ctx, string(data), text)
}

func (r *syncRun) progressText() string {
	switch r.c.Phase {
	case phaseNew:
		return fmt.Sprintf(texts.TgProgressNew, r.c.Loaded)
	case phaseOld:
		return fmt.Sprintf(texts.TgProgressOld, r.c.Loaded, r.c.Offset)
	case phaseDeleted:
		return fmt.Sprintf(texts.TgProgressDeleted, r.c.Verified)
	default:
		return fmt.Sprintf(texts.TgProgressDone, r.c.Loaded)
	}
}
