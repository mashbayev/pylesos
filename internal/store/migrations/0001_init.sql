-- Файл: internal/store/migrations/0001_init.sql
-- Каркас (M1): настройки, источники, задания. Время — Unix-секунды.
CREATE TABLE settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

CREATE TABLE sources (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    kind       TEXT    NOT NULL CHECK (kind IN ('telegram_group', 'site')),
    title      TEXT    NOT NULL,
    config     TEXT    NOT NULL DEFAULT '{}',
    created_at INTEGER NOT NULL
);

CREATE TABLE jobs (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    source_id     INTEGER NOT NULL REFERENCES sources (id),
    kind          TEXT    NOT NULL,
    params        TEXT    NOT NULL DEFAULT '{}',
    status        TEXT    NOT NULL CHECK (status IN
                  ('queued', 'running', 'paused', 'waiting_user', 'done', 'failed', 'cancelled')),
    cursor        TEXT    NOT NULL DEFAULT '{}',
    progress_text TEXT    NOT NULL DEFAULT '',
    reason        TEXT    NOT NULL DEFAULT '',
    created_at    INTEGER NOT NULL,
    updated_at    INTEGER NOT NULL,
    finished_at   INTEGER
);

-- Одно активное задание на источник (спецификация, раздел 3, «Общее»).
CREATE UNIQUE INDEX jobs_one_active_per_source ON jobs (source_id)
    WHERE status IN ('queued', 'running', 'paused', 'waiting_user');
