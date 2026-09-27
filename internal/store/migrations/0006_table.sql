-- Файл: internal/store/migrations/0006_table.sql
-- M5: колонки таблицы (конструктор, П29) и готовые строки людей (кэш: модель не гоняется повторно).
CREATE TABLE table_config (
    id              INTEGER PRIMARY KEY CHECK (id = 1),
    columns         TEXT    NOT NULL, -- []tables.Column, JSON
    include_queries INTEGER NOT NULL DEFAULT 0 -- П20: люди только из запросов
);

CREATE TABLE table_rows (
    person_id INTEGER PRIMARY KEY, -- без внешнего ключа: склейка удаляет человека, строка просто устаревает
    hash      TEXT    NOT NULL,    -- отпечаток колонок и входных текстов
    row       TEXT    NOT NULL     -- tables.Row, JSON
);
