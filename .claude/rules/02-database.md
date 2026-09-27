# 02 — База данных

> Подробности по таблицам (колонки, индексы, история решений) — `docs/reference/db-schema.md`.
> **Перед работой с конкретной таблицей — прочитай её строку там.** Полные DDL — `internal/store/migrations/`.

## Библиотеки
- SQLite на чистом Go (`modernc.org/sqlite`, без CGO — П14) + `github.com/jmoiron/sqlx`
- ЗАПРЕЩЕНО: GORM и любые другие ORM
- `GetContext` — одна запись, `SelectContext` — слайс, `NamedExecContext` — INSERT/UPDATE
- Транзакции: `tx, err := db.BeginTxx(ctx, nil)`; откат — `errors.Join(err, tx.Rollback())`, не `defer tx.Rollback()` с проглоченной ошибкой
- Одно соединение на запись (`SetMaxOpenConns(1)`), WAL, `foreign_keys(1)`, `busy_timeout`

## Паттерн Repository
- Вся работа с БД — за интерфейсами репозитория (`internal/<модуль>/repository/`)
- Хэндлеры и сервисы НЕ знают о SQL

## Схема
- Только миграциями (`internal/store/migrations/NNNN_название.sql`), применяются при запуске
- Время — `INTEGER` (Unix-секунды), значение ставит Go, не SQL
- Пустые списки из репозитория — `[]`, не `nil`

## Грабли (обязательно помнить)
- Слияние при повторном сборе/импорте — только COALESCE: пустое значение никогда не затирает сохранённое.
- Вычисляемые флаги не хранятся — считаются в репозитории/сервисе при чтении.
- Секреты в БД (сессия Telegram, токен бота) — AES-256-GCM, в JSON не отдавать (`json:"-"`).
- Одно активное задание на источник — частичный уникальный индекс `jobs_one_active_per_source`;
  нарушение → `domain.ErrSourceBusy`.

## Таблицы (подробно — `docs/reference/db-schema.md`)

| Таблица | Этап | Назначение |
|---|---|---|
| `settings` | M1 | настройки ключ → значение |
| `sources` | M1 | источники: Telegram-группа или площадка |
| `jobs` | M1 | задания сбора и разбора, курсор продолжения |
