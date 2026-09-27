-- Файл: internal/store/migrations/0002_telegram.sql
-- M2: секреты, сообщения и вложения Telegram. Время — Unix-секунды, ставит Go.
CREATE TABLE secrets (
    key   TEXT PRIMARY KEY,
    value BLOB NOT NULL -- nonce + шифротекст AES-256-GCM
);

-- Один источник на группу: повторное «Добавить» не создаёт второй.
CREATE UNIQUE INDEX sources_kind_config ON sources (kind, config);

CREATE TABLE tg_messages (
    source_id   INTEGER NOT NULL REFERENCES sources (id),
    msg_id      INTEGER NOT NULL,
    date        INTEGER NOT NULL,
    author_id   INTEGER NOT NULL DEFAULT 0,
    author_name TEXT    NOT NULL DEFAULT '',
    text        TEXT    NOT NULL DEFAULT '',
    reply_to    INTEGER NOT NULL DEFAULT 0,
    thread_top  INTEGER NOT NULL DEFAULT 0,
    fwd_from    TEXT    NOT NULL DEFAULT '',
    edit_date   INTEGER NOT NULL DEFAULT 0,
    deleted     INTEGER NOT NULL DEFAULT 0,
    via         TEXT    NOT NULL CHECK (via IN ('account', 'bot')),
    raw         TEXT    NOT NULL DEFAULT '',
    fetched_at  INTEGER NOT NULL,
    PRIMARY KEY (source_id, msg_id)
);

CREATE TABLE tg_media (
    source_id       INTEGER NOT NULL,
    msg_id          INTEGER NOT NULL,
    idx             INTEGER NOT NULL,
    kind            TEXT    NOT NULL CHECK (kind IN ('photo', 'document')),
    file_id         INTEGER NOT NULL,
    mime_type       TEXT    NOT NULL DEFAULT '',
    file_name       TEXT    NOT NULL DEFAULT '',
    size            INTEGER NOT NULL DEFAULT 0,
    status          TEXT    NOT NULL DEFAULT 'pending'
                    CHECK (status IN ('pending', 'downloaded', 'recognized', 'deleted')),
    recognized_text TEXT    NOT NULL DEFAULT '',
    model           TEXT    NOT NULL DEFAULT '',
    PRIMARY KEY (source_id, msg_id, idx),
    FOREIGN KEY (source_id, msg_id) REFERENCES tg_messages (source_id, msg_id)
);
