# doc-agent — что копируем (П5)

Источник: `~/doc-agent`, коммит `46c6a3f` (v1.3.0-7). Go 1.26, UI — ванильный JS, вся логика в Go.

## Пакеты-кандидаты на копию

| Пакет | Что внутри |
|---|---|
| `internal/tables` | модель шаблона `Template{Name, Mode, Instructions, Columns, Forms}`, `Column{Name, Type, Hint, Required, Options}`; типы колонок text/number/money/date/iin/bin/phone/bool/choice/comment; режимы строк document/table_rows/page/people; зоны на образце; хранилище шаблонов (`store.go`) |
| `internal/export` | Excel через excelize, подсветка сомнительных ячеек, Markdown |
| `internal/validate` | проверки ИИН/БИН (контрольная цифра), дат на русском и казахском, сумм, телефонов |
| `internal/normalize` | входные форматы: JPG, PNG, PDF, TIFF, DOCX |
| `internal/recognize` | транскрипция, извлечение полей по шаблону, поиск карточки документа на скрине, зоны, исправление гомоглифов |
| `internal/ollama` | клиент Ollama (`gemma4:e4b`) |
| `internal/ui/server.go` | HTTP API конструктора: `/api/templates/*` (save, delete, export, import, suggest, probe, sample) — как образец API, фронт пишется заново на React |

## Чего не хватает модели шаблона для Пылесоса

- Режимы строк: «человек = строка» со склейкой упоминаний из разных сообщений, «сообщение = строка»,
  «объявление = строка».
- У колонки — **источник значения**: текст/подпись сообщения, распознанный документ, ответы и комментарии,
  служебные поля (автор, дата, ссылка на сообщение/объявление).

## Соответствие правилам кода (П15)

Скопированный код приводится к правилам `.claude/rules/` (прежде всего `07-clean-code.md`) при копировании. Замечено заранее:
в doc-agent есть переменные уровня пакета для подмены в тестах (`openWindow`, `minWindowLife`, `jobBusy` в `internal/ui/window.go`)
— это глобальное состояние, в Пылесосе заменяется зависимостями через конструктор.

## Перенесено в M3 (2026-09-27)

| doc-agent | Пылесос | Что переделано |
|---|---|---|
| `core.Request/Responder` | `internal/llm` | ChatGPT Web не переносится, только тип запроса и ошибки |
| `ollama` | `internal/ollama` | тело ответа и ошибки не глотаются; срок в `VisionModels` — ошибка |
| `normalize` | `internal/normalize` | байты вместо папок (`Normalizer.Bytes`); pdfium — поле структуры; TIFF/DOCX разбиты |
| `tables` (модель) | `internal/tables` | глобальные срезы → функции; `Validate` разбит; YAML — `yaml.go`; файлового хранилища нет |
| `validate` | `internal/validate` | регулярки/карты → функции; тексты `chk.*` — `texts.CheckIssue` |
| `recognize` | `internal/recognize` | `Core`→`LLM`; `extract.go` → 3 файла; `Locate` разбит |
| `export/excel.go` | `internal/export` | переписан: все ошибки проверяются; только русский |
| зоны шаблона | `internal/docforms` | зоны принадлежат форме документа (П21), поля постоянные |

