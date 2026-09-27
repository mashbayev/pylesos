// Файл: internal/telegram/repository/sync_state.go
package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// SyncState — сверка группы аккаунтом: SyncedTop — всё до него загружено без пропусков;
// PassHead/PassLow — незавершённый проход «новые сообщения» (PassHead == 0 — прохода нет).
type SyncState struct {
	SyncedTop int `db:"synced_top"`
	PassHead  int `db:"pass_head"`
	PassLow   int `db:"pass_low"`
}

func (r *SQLite) SyncState(ctx context.Context, sourceID int64) (SyncState, error) {
	var st SyncState
	err := r.db.GetContext(ctx, &st, `SELECT synced_top, pass_head, pass_low FROM tg_sync_state WHERE source_id = ?`, sourceID)
	if errors.Is(err, sql.ErrNoRows) {
		return SyncState{}, nil
	}
	if err != nil {
		return SyncState{}, fmt.Errorf("состояние сверки группы %d: %w", sourceID, err)
	}
	return st, nil
}

func (r *SQLite) SaveSyncState(ctx context.Context, sourceID int64, st SyncState) error {
	if _, err := r.db.ExecContext(ctx, `INSERT INTO tg_sync_state (source_id, synced_top, pass_head, pass_low)
		VALUES (?, ?, ?, ?) ON CONFLICT (source_id) DO UPDATE SET
		synced_top = excluded.synced_top, pass_head = excluded.pass_head, pass_low = excluded.pass_low`,
		sourceID, st.SyncedTop, st.PassHead, st.PassLow); err != nil {
		return fmt.Errorf("сохранить состояние сверки группы %d: %w", sourceID, err)
	}
	return nil
}

// RecentWindowStart — номер, после которого начинаются n последних живых сообщений (0 — вся история короче n).
func (r *SQLite) RecentWindowStart(ctx context.Context, sourceID int64, n int) (int, error) {
	ids := []int{}
	if err := r.db.SelectContext(ctx, &ids, `SELECT msg_id FROM tg_messages WHERE source_id = ? AND deleted = 0
		ORDER BY msg_id DESC LIMIT 1 OFFSET ?`, sourceID, n); err != nil {
		return 0, fmt.Errorf("окно проверки удалённых: %w", err)
	}
	if len(ids) == 0 {
		return 0, nil
	}
	return ids[0], nil
}
