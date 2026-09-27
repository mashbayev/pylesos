-- Файл: internal/store/migrations/0005_people.sql
-- M4: разбор Telegram — люди, упоминания, кандидаты на склейку; отметки разбора сообщений и полей документов.
ALTER TABLE tg_messages ADD COLUMN analyzed INTEGER NOT NULL DEFAULT 0; -- 0 ждёт, 1 разобрано, 2 ошибка модели
ALTER TABLE tg_messages ADD COLUMN analyze_error TEXT NOT NULL DEFAULT '';
ALTER TABLE tg_media ADD COLUMN fields TEXT NOT NULL DEFAULT '{}'; -- опознавательные поля документа (JSON)

CREATE TABLE persons (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    full_name  TEXT    NOT NULL,
    name_key   TEXT    NOT NULL, -- нормализованное ФИО (people/names)
    iin        TEXT,             -- NULL — неизвестен
    doc_number TEXT    NOT NULL DEFAULT '',
    birth_date TEXT    NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL
);
CREATE UNIQUE INDEX persons_iin ON persons (iin) WHERE iin IS NOT NULL;
CREATE INDEX persons_name_key ON persons (name_key);

CREATE TABLE mentions (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    source_id       INTEGER NOT NULL,
    msg_id          INTEGER NOT NULL,
    person_id       INTEGER REFERENCES persons (id), -- NULL — не привязано
    role            TEXT    NOT NULL CHECK (role IN ('record', 'query', 'comment')),
    name_as_written TEXT    NOT NULL DEFAULT '',
    link            TEXT    NOT NULL CHECK (link IN ('iin', 'name', 'document', 'reply', 'manual', 'none')),
    FOREIGN KEY (source_id, msg_id) REFERENCES tg_messages (source_id, msg_id)
);
CREATE INDEX mentions_person ON mentions (person_id);
CREATE INDEX mentions_message ON mentions (source_id, msg_id);

CREATE TABLE merge_candidates (
    a      INTEGER NOT NULL REFERENCES persons (id),
    b      INTEGER NOT NULL REFERENCES persons (id),
    status TEXT    NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'merged', 'rejected')),
    PRIMARY KEY (a, b)
);
