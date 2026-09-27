-- Файл: internal/store/migrations/0004_doc_forms.sql
-- Формы документов (П21): образец, отпечаток, рамка карточки и зоны опознавательных полей.
CREATE TABLE doc_forms (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    name       TEXT    NOT NULL,
    sample     BLOB    NOT NULL, -- JPEG образца (нужен для разметки)
    signature  TEXT    NOT NULL, -- отпечаток образца 32×32 (recognize.Signature)
    frame      TEXT    NOT NULL DEFAULT '', -- JSON tables.Zone или ''
    locator    TEXT    NOT NULL DEFAULT '', -- образец карточки для поиска на скрине (recognize.Locator)
    zones      TEXT    NOT NULL DEFAULT '{}', -- JSON: поле → tables.Zone
    updated_at INTEGER NOT NULL
);
