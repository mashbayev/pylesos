// Файл: internal/screens/types.go
// Package screens собирает экраны для тонкого фронта (П3): тексты, статусы, кнопки — всё готовое.
package screens

type NavItem struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

type Layout struct {
	AppName    string    `json:"app_name"`
	Nav        []NavItem `json:"nav"`
	ThemeLabel string    `json:"theme_label"`
	QuitLabel  string    `json:"quit_label"`
	ClosedText string    `json:"closed_text"` // показывается после «Выхода»
}

// Action — кнопка. Tone: "primary" | "danger" | "default".
type Action struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Tone  string `json:"tone"`
}

type Banner struct {
	JobID int64  `json:"job_id"`
	Text  string `json:"text"`
}

type HomeScreen struct {
	Title         string      `json:"title"`
	Banners       []Banner    `json:"banners"`
	EmptyText     string      `json:"empty_text"` // пусто, когда источники есть
	TelegramTitle string      `json:"telegram_title"`
	Telegram      []SourceRow `json:"telegram"`
}

// SourceRow — источник на главной: строки сведений и кнопки.
type SourceRow struct {
	ID      int64    `json:"id"`
	Title   string   `json:"title"`
	Lines   []string `json:"lines"`
	Actions []Action `json:"actions"`
}

// TelegramScreen — вход по шагам; после входа — группы аккаунта.
type TelegramScreen struct {
	Title         string     `json:"title"`
	Hint          string     `json:"hint"`
	Fields        []Field    `json:"fields"` // поля текущего шага; пусто, когда вход выполнен
	SubmitLabel   string     `json:"submit_label"`
	SavingLabel   string     `json:"saving_label"`
	Status        string     `json:"status"`
	Message       string     `json:"message"`
	GroupsTitle   string     `json:"groups_title"`
	GroupsEmpty   string     `json:"groups_empty"`
	RefreshLabel  string     `json:"refresh_label"`
	EditKeysLabel string     `json:"edit_keys_label"` // «Изменить api_id и api_hash»; пусто — кнопки нет
	Groups        []GroupRow `json:"groups"`
	BotTitle      string     `json:"bot_title"`
	BotHint       string     `json:"bot_hint"`
	BotStatus     string     `json:"bot_status"`
	BotSaveLabel  string     `json:"bot_save_label"`
	BotField      Field      `json:"bot_field"`
}

// GroupRow — группа аккаунта; Action nil — уже добавлена (тогда Status).
type GroupRow struct {
	Key    string  `json:"key"`
	Title  string  `json:"title"`
	Status string  `json:"status"`
	Action *Action `json:"action"`
}

// JobRow — строка экрана «Задания». StatusTone: "neutral" | "running" | "warning" | "success" | "danger".
type JobRow struct {
	ID           int64    `json:"id"`
	Title        string   `json:"title"`
	StatusText   string   `json:"status_text"`
	StatusTone   string   `json:"status_tone"`
	ProgressText string   `json:"progress_text"`
	Reason       string   `json:"reason"`
	UpdatedText  string   `json:"updated_text"`
	Actions      []Action `json:"actions"`
}

type JobsScreen struct {
	Title     string   `json:"title"`
	EmptyText string   `json:"empty_text"`
	Jobs      []JobRow `json:"jobs"`
}

// Field — поле формы. Kind: "text" | "number" | "password" (подсказка для отрисовки, не для проверки).
type Field struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Value string `json:"value"`
	Kind  string `json:"kind"`
	Error string `json:"error"`
}

type SettingsScreen struct {
	Title        string  `json:"title"`
	OllamaLabel  string  `json:"ollama_label"`
	OllamaStatus string  `json:"ollama_status"`
	Fields       []Field `json:"fields"`
	SaveLabel    string  `json:"save_label"`
	SavingLabel  string  `json:"saving_label"`
	Message      string  `json:"message"`
}

// FormsScreen — формы документов: список и редактор выбранной формы (Editor == nil — не выбрана).
type FormsScreen struct {
	Title       string      `json:"title"`
	Hint        string      `json:"hint"`
	EmptyText   string      `json:"empty_text"`
	NameLabel   string      `json:"name_label"`
	FileLabel   string      `json:"file_label"`
	CreateLabel string      `json:"create_label"`
	SavingLabel string      `json:"saving_label"`
	Message     string      `json:"message"`
	Forms       []FormRow   `json:"forms"`
	Editor      *FormEditor `json:"editor"`
}

type FormRow struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	Selected  bool   `json:"selected"`
	OpenLabel string `json:"open_label"`
}

// FormEditor — образец и отметки; координаты — доли ширины и высоты образца (0…1).
type FormEditor struct {
	ID          int64        `json:"id"`
	Name        string       `json:"name"`
	SampleURL   string       `json:"sample_url"`
	Hint        string       `json:"hint"`
	Targets     []ZoneTarget `json:"targets"`
	Boxes       []ZoneBox    `json:"boxes"`
	ClearLabel  string       `json:"clear_label"`
	RenameLabel string       `json:"rename_label"`
	DeleteLabel string       `json:"delete_label"`
}

// ZoneTarget — что можно отметить: рамка карточки или поле.
type ZoneTarget struct {
	Key    string `json:"key"`
	Label  string `json:"label"`
	Marked bool   `json:"marked"`
	Color  string `json:"color"`
}

type ZoneBox struct {
	Key   string  `json:"key"`
	Label string  `json:"label"`
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
	W     float64 `json:"w"`
	H     float64 `json:"h"`
	Color string  `json:"color"`
}

// PeopleScreen — люди: вкладки «все / возможно, один человек / не привязано»; Card — открытый человек.
type PeopleScreen struct {
	Title       string         `json:"title"`
	Tab         string         `json:"tab"`
	Tabs        []NavItem      `json:"tabs"`
	Query       string         `json:"query"`
	SearchLabel string         `json:"search_label"`
	EmptyText   string         `json:"empty_text"`
	Message     string         `json:"message"`
	OpenLabel   string         `json:"open_label"`
	Rows        []SourceRow    `json:"rows"`
	Card        *PersonCard    `json:"card"`
	Candidates  []CandidateRow `json:"candidates"`
	Unlinked    []UnlinkedRow  `json:"unlinked"`
}

type PersonCard struct {
	ID       int64        `json:"id"`
	Name     string       `json:"name"`
	Fields   []Field      `json:"fields"`
	Mentions []MentionRow `json:"mentions"`
}

// MentionRow — упоминание: сообщение группы; Link пусто — ссылки нет.
type MentionRow struct {
	Role   string `json:"role"`
	Date   string `json:"date"`
	Group  string `json:"group"`
	Author string `json:"author"`
	Text   string `json:"text"`
	Link   string `json:"link"`
}

type PersonBrief struct {
	ID    int64    `json:"id"`
	Name  string   `json:"name"`
	Lines []string `json:"lines"`
}

type CandidateRow struct {
	A       PersonBrief `json:"a"`
	B       PersonBrief `json:"b"`
	Actions []Action    `json:"actions"`
}

// Option — вариант выбора; ID 0 — «новый человек».
type Option struct {
	ID    int64  `json:"id"`
	Label string `json:"label"`
}

type UnlinkedRow struct {
	ID        int64      `json:"id"`
	Mention   MentionRow `json:"mention"`
	Name      string     `json:"name"`
	Options   []Option   `json:"options"`
	LinkLabel string     `json:"link_label"`
}

// TableScreen — конструктор колонок таблицы Excel (П29).
type TableScreen struct {
	Title          string        `json:"title"`
	Hint           string        `json:"hint"`
	Columns        []TableColumn `json:"columns"`
	NewColumn      TableColumn   `json:"new_column"`
	Types          []Option2     `json:"types"`
	NameLabel      string        `json:"name_label"`
	TypeLabel      string        `json:"type_label"`
	HintLabel      string        `json:"hint_label"`
	AddLabel       string        `json:"add_label"`
	RemoveLabel    string        `json:"remove_label"`
	IncludeQueries bool          `json:"include_queries"`
	IncludeLabel   string        `json:"include_label"`
	SaveLabel      string        `json:"save_label"`
	SavingLabel    string        `json:"saving_label"`
	Message        string        `json:"message"`
	Error          string        `json:"error"`
}

type TableColumn struct {
	Name string `json:"name"`
	Type string `json:"type"`
	Hint string `json:"hint"`
}

// Option2 — вариант выбора со строковым значением.
type Option2 struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}
