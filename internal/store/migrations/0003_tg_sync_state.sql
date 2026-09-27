-- Файл: internal/store/migrations/0003_tg_sync_state.sql
-- Состояние сверки группы аккаунтом (ревью M2, C1): до какого номера история загружена без пропусков
-- и незавершённый проход «новые сообщения» — чтобы новое задание продолжило его, а не оставило дыру.
CREATE TABLE tg_sync_state (
    source_id  INTEGER PRIMARY KEY REFERENCES sources (id),
    synced_top INTEGER NOT NULL DEFAULT 0, -- всё до этого номера загружено аккаунтом
    pass_head  INTEGER NOT NULL DEFAULT 0, -- верх незавершённого прохода (0 — прохода нет)
    pass_low   INTEGER NOT NULL DEFAULT 0  -- докуда проход дошёл вниз
);
