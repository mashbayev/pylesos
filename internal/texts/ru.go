// Файл: internal/texts/ru.go
// Package texts — все тексты интерфейса. Только русский (П22); фронт получает их от бэкенда.
package texts

// Оболочка окна.
const (
	AppName     = "Пылесос"
	NavHome     = "Источники"
	NavJobs     = "Задания"
	NavSettings = "Настройки"
	ThemeToggle = "Тема"
	Quit        = "Выход"
)

// Экран «Источники».
const (
	HomeTitle     = "Источники"
	HomeEmpty     = "Источников пока нет. Здесь появятся группы Telegram и поиски OLX."
	BannerWaiting = "«%s» — %s: %s" // источник, вид задания, причина
)

// Экран «Задания».
const (
	JobsTitle         = "Задания"
	JobsEmpty         = "Заданий пока нет."
	JobTitle          = "%s — %s" // вид задания, источник
	StatusQueued      = "В очереди"
	StatusRunning     = "Идёт"
	StatusPaused      = "На паузе"
	StatusWaitingUser = "Ждёт вас"
	StatusDone        = "Готово"
	StatusFailed      = "Ошибка"
	StatusCancelled   = "Отменено"
	ActionPause       = "Пауза"
	ActionResume      = "Продолжить"
	ActionCancel      = "Отменить"
	JobUnknownKind    = "Этот вид задания больше не поддерживается программой"
	JobCrashed        = "Внутренний сбой исполнителя задания"
)

// Экран «Настройки».
const (
	SettingsTitle         = "Настройки"
	SettingsExportsDir    = "Папка для выгрузок Excel"
	SettingsPhonePause    = "Пауза между открытиями телефонов OLX, секунд"
	SettingsSave          = "Сохранить"
	Saving                = "Сохранение…"
	SettingsSaved         = "Настройки сохранены"
	ErrExportsDirEmpty    = "Укажите папку для выгрузок"
	ErrExportsDirRelative = "Укажите полный путь к папке, например /home/имя/Документы/Пылесос"
	ErrPauseNotNumber     = "Введите целое число секунд"
	ErrPauseRange         = "Пауза должна быть от %d до %d секунд"
)

// Ошибки действий и запросов.
const (
	ErrActionUnavailable = "Это действие сейчас недоступно — обновите экран"
	ErrSourceBusy        = "По этому источнику уже есть активное задание"
	ErrJobNotFound       = "Задание не найдено"
	ErrBadRequest        = "Неверный запрос"
	ErrInternal          = "Внутренняя ошибка программы, подробности в журнале"
	ErrForbidden         = "Нет доступа к программе. Закройте это окно и запустите Пылесос заново."
	ProgramClosed        = "Программа закрыта. Это окно можно закрыть."
)

// Telegram (M2).
const (
	NavTelegram       = "Telegram"
	TgTitle           = "Telegram"
	TgKeysHint        = "Получите api_id и api_hash на my.telegram.org → «API development tools» и введите их здесь."
	TgPhoneHint       = "Введите номер телефона вашего аккаунта Telegram."
	TgCodeHint        = "Telegram прислал код в приложение (или SMS) на номер %s."
	TgPasswordHint    = "У аккаунта включена двухэтапная проверка — введите облачный пароль."
	TgAPIID           = "api_id"
	TgAPIHash         = "api_hash"
	TgPhone           = "Номер телефона"
	TgCode            = "Код из Telegram"
	TgPassword        = "Облачный пароль"
	TgNext            = "Далее"
	TgLoggedIn        = "Вход выполнен: %s"
	TgGroupsTitle     = "Ваши группы"
	TgGroupsEmpty     = "Групп не найдено."
	TgRefresh         = "Обновить список"
	TgAddGroup        = "Добавить"
	TgAdded           = "Добавлена"
	TgGroupAdded      = "Группа добавлена, сбор начат"
	TgSyncKind        = "Сбор Telegram"
	TgCollect         = "Собрать"
	TgMessagesCount   = "Сообщений: %d"
	TgLastSync        = "Последний сбор: %s"
	TgNeverSynced     = "Ещё не собиралась"
	TgNeedLogin       = "Войдите в Telegram на вкладке «Telegram»"
	TgFloodWait       = "Telegram попросил подождать %d с"
	TgProgressNew     = "Новые сообщения: загружено %d"
	TgProgressOld     = "История: загружено %d, дошли до сообщения №%d"
	TgProgressDeleted = "Проверка удалённых: проверено %d"
	TgProgressDone    = "Готово: загружено %d"
	HomeTelegramTitle = "Группы Telegram"
	ErrTgAPIID        = "api_id — это число, например 1234567"
	ErrTgAPIHash      = "api_hash — 32 символа: цифры и буквы a–f"
	ErrTgPhone        = "Введите номер в международном формате, например +77011234567"
	ErrTgBadPhone     = "Telegram не принял этот номер"
	ErrTgBadCode      = "Неверный код"
	ErrTgCodeExpired  = "Код устарел — введите номер ещё раз"
	ErrTgBadPassword  = "Неверный облачный пароль"
	ErrTgGroupGone    = "Группа не найдена — обновите список"
)

// Telegram: ошибки связи и доступа (ревью M2).
const (
	TgNoConnection      = "Нет связи с Telegram. Проверьте интернет — программа повторит попытку сама."
	TgNoConnectionRetry = "Нет связи с Telegram — повторю через %d с"
	TgGroupUnavailable  = "Нет доступа к группе — проверьте, что ваш аккаунт в ней состоит, и нажмите «Продолжить»"
	TgSyncFailed        = "Сбой сбора Telegram — нажмите «Продолжить»"
	TgEditKeys          = "Изменить api_id и api_hash"
	ErrTgNoAccount      = "На этот номер нет аккаунта Telegram"
	ErrTgBadKeys        = "Telegram не принял api_id или api_hash — проверьте и введите заново"
)

// Telegram-бот (M2.1).
const (
	TgBotTitle          = "Бот для новых сообщений"
	TgBotHint           = "Создайте бота у @BotFather, вставьте сюда его токен и добавьте бота в каждую группу администратором — тогда новые сообщения будут приходить сразу, пока программа открыта."
	TgBotToken          = "Токен бота"
	TgBotSave           = "Сохранить токен"
	TgBotOff            = "Бот не подключён"
	TgBotStarting       = "Бот подключается…"
	TgBotRunning        = "Бот @%s принимает новые сообщения"
	TgBotBadToken       = "Telegram не принял токен бота — проверьте его и сохраните заново"
	TgBotConflict       = "Этого бота уже слушает другая программа (вебхук или вторая копия) — повторю через минуту"
	TgBotOffline        = "Нет связи с Telegram — бот повторит попытку сам"
	TgBotSaved          = "Токен сохранён, бот подключается"
	ErrTgBotTokenFormat = "Токен выглядит так: 123456789:AAE… — скопируйте его из @BotFather целиком"
)

// Excel (перенос из doc-agent, M3).
const (
	XlsxSheetData   = "Данные"
	XlsxSheetReport = "Отчёт"
	XlsxFile        = "Файл"
	XlsxPage        = "Страница"
	XlsxReview      = "Требует проверки"
	XlsxPages       = "Страниц"
	XlsxStatus      = "Статус"
	XlsxError       = "Ошибка"
	XlsxStatusOK    = "готово"
	XlsxStatusError = "ошибка"
	ErrXlsxLocked   = "Не удалось записать таблицу Excel — закройте её в Excel и запустите снова"
)

// CheckIssue — текст замечания проверки значения по ключу validate (chk.*); неизвестный ключ — как есть.
func CheckIssue(key string) string {
	switch key {
	case "chk.required":
		return "не заполнено обязательное поле"
	case "chk.unclear":
		return "есть неразборчивый фрагмент"
	case "chk.mixed_script":
		return "смешаны кириллица и латиница"
	case "chk.number":
		return "не число"
	case "chk.date":
		return "не удалось распознать дату"
	case "chk.iin_format":
		return "ИИН должен состоять из 12 цифр с корректной датой рождения"
	case "chk.iin_checksum":
		return "неверная контрольная цифра ИИН"
	case "chk.bin_format":
		return "БИН должен состоять из 12 цифр"
	case "chk.bin_checksum":
		return "неверная контрольная цифра БИН"
	case "chk.phone":
		return "не похоже на номер телефона"
	case "chk.bool":
		return "ожидалось «да» или «нет»"
	case "chk.choice":
		return "значение не из списка вариантов"
	case "chk.low_confidence":
		return "модель не уверена в значении"
	case "chk.number_ambiguous":
		return "непонятный разделитель: тысячи или дробная часть — сверьте с оригиналом"
	default:
		return key
	}
}

// Формы документов (M3, П21).
const (
	NavForms             = "Формы документов"
	FormsTitle           = "Формы документов"
	FormsHint            = "Загрузите образец документа (удостоверение, права), отметьте мышью рамку карточки и места ФИО, ИИН, номера и даты рождения — разбор будет читать именно эти места."
	FormsEmpty           = "Форм пока нет."
	FormsName            = "Название формы"
	FormsFile            = "Образец (фото, скан или PDF)"
	FormsCreate          = "Добавить форму"
	FormsOpen            = "Открыть"
	FormsStatus          = "Отмечено полей: %d из %d, рамка: %s"
	FormsFrameYes        = "есть"
	FormsFrameNo         = "нет"
	FormsFrame           = "Рамка карточки"
	FormsEditorHint      = "Выберите, что отмечать, и обведите это место на образце мышью."
	FormsClear           = "Снять отметку"
	FormsRename          = "Переименовать"
	FormsDelete          = "Удалить форму"
	FormsCreated         = "Форма добавлена — отметьте рамку и поля"
	FormsSaved           = "Отметка сохранена"
	FormsDeleted         = "Форма удалена"
	ErrFormsNoName       = "Введите название формы"
	ErrFormsNoSample     = "Образец не читается — нужен JPG, PNG, TIFF или PDF"
	ErrFormsPDFProtected = "PDF защищён паролем — снимите пароль или сохраните страницу картинкой"
	ErrFormsUnsupported  = "Такой файл не подходит — нужен JPG, PNG, TIFF или PDF"
	ErrFormsBadZone      = "Отметка должна быть внутри образца"
	ErrFormsBadField     = "Такого поля у формы нет"
	ErrFormsNotFound     = "Форма не найдена — обновите экран"
)

// Ollama в «Настройках» (M3).
const (
	SettingsOllama = "Модель распознавания (Ollama)"
	OllamaOK       = "Ollama работает, модель %s установлена"
	OllamaDown     = "Ollama не запущена — установите её с ollama.com и запустите"
	OllamaNoModel  = "Модель не установлена — выполните в терминале: ollama pull gemma4:e4b"
)

// Разбор Telegram (M4).
const (
	AnalyzeKind      = "Разбор Telegram"
	AnalyzeNeedModel = "Запустите Ollama и установите модель (ollama pull gemma4:e4b), затем нажмите «Продолжить»"
	AnalyzeBadAnswer = "не разобрано: ошибка модели"
	AnalyzeDocs      = "Документы: распознано %d"
	AnalyzeMsgs      = "Сообщения: разобрано %d"
	AnalyzeDone      = "Разбор завершён"
	AnalyzeFailed    = "Сбой разбора — нажмите «Продолжить»"
	AnalyzeCollect   = "Разобрать"
)

// Экран «Люди» (M4).
const (
	NavPeople         = "Люди"
	PeopleTitle       = "Люди"
	PeopleSearch      = "Поиск по ФИО или ИИН"
	PeopleTabAll      = "Все"
	PeopleTabCands    = "Возможно, один человек"
	PeopleTabUnlinked = "Не привязано"
	PeopleEmpty       = "Пока никого нет — нажмите «Разобрать» у группы на главной."
	PeopleCandsEmpty  = "Похожих пар нет."
	PeopleLooseEmpty  = "Все упоминания привязаны."
	PeopleOpen        = "Открыть"
	PeopleCounts      = "Записей: %d, запросов: %d, комментариев: %d"
	PeopleGroups      = "Группы: %s"
	PeopleIIN         = "ИИН"
	PeopleDoc         = "Номер документа"
	PeopleBirth       = "Дата рождения"
	PeopleUnknown     = "не известен"
	PeopleMerge       = "Это один человек"
	PeopleReject      = "Разные люди"
	PeopleLink        = "Привязать"
	PeopleNewPerson   = "Новый человек: %s"
	PeopleMerged      = "Склеено"
	PeopleRejected    = "Отмечено: разные люди"
	PeopleLinked      = "Привязано"
	RoleRecord        = "запись"
	RoleQuery         = "запрос"
	RoleComment       = "комментарий"
	ErrPeopleNoName   = "У упоминания нет имени — выберите человека из списка"
	ErrPeopleNotFound = "Человек не найден — обновите экран"
)

// Таблица Excel по людям (M5, П28, П29).
const (
	NavTable            = "Таблица"
	TableTitle          = "Таблица Excel"
	TableHint           = "Колонки, которые модель заполнит по документам каждого человека. Выгрузка — кнопка «В Excel» у группы на главной."
	TableColName        = "Колонка"
	TableColType        = "Тип"
	TableColHint        = "Подсказка: где искать"
	TableAddColumn      = "Добавить колонку"
	TableRemove         = "Убрать"
	TableIncludeQueries = "Включать людей только из запросов «есть инфо?»"
	TableSave           = "Сохранить"
	TableSaving         = "Сохранение…"
	TableSaved          = "Сохранено"
	TableComments       = "Комментарии"
	TableExport         = "В Excel"
	ExportKind          = "выгрузка в Excel"
	ExportPeople        = "Люди: %d из %d"
	ExportDone          = "Готово: %s"
	ExportFailed        = "Выгрузка не удалась"
	ExportNoColumns     = "Добавьте колонки на экране «Таблица»"
	ErrTableNoColumns   = "Добавьте хотя бы одну колонку"
	ErrTableColNoName   = "У колонки %s нет названия"
	ErrTableColDup      = "Колонка «%s» повторяется"
	ErrTableColReserved = "Название «%s» занято служебной колонкой"
	ErrTableColType     = "У колонки «%s» неверный тип"
	TypeText            = "Текст"
	TypeNumber          = "Число"
	TypeDate            = "Дата"
	TypeIIN             = "ИИН"
	TypePhone           = "Телефон"
)
