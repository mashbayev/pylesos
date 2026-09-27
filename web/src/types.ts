// Файл: web/src/types.ts
// Формы экранов — зеркало internal/screens/types.go.
export interface NavItem { id: string; label: string }
export interface Layout { app_name: string; nav: NavItem[]; theme_label: string; quit_label: string; closed_text: string }
export interface Action { id: string; label: string; tone: 'primary' | 'danger' | 'default' }
export interface Banner { job_id: number; text: string }
export interface JobRow {
  id: number
  title: string
  status_text: string
  status_tone: 'neutral' | 'running' | 'warning' | 'success' | 'danger'
  progress_text: string
  reason: string
  updated_text: string
  actions: Action[]
}
export interface JobsScreen { title: string; empty_text: string; jobs: JobRow[] }
export interface SettingsScreen { title: string; ollama_label: string; ollama_status: string; fields: Field[]; save_label: string; saving_label: string; message: string }
export interface Field { key: string; label: string; value: string; kind: 'text' | 'number' | 'password'; error: string }
export interface SourceRow { id: number; title: string; lines: string[]; actions: Action[] }
export interface HomeScreen { title: string; banners: Banner[]; empty_text: string; telegram_title: string; telegram: SourceRow[] }
export interface GroupRow { key: string; title: string; status: string; action: Action | null }
export interface TelegramScreen {
  title: string
  hint: string
  fields: Field[]
  submit_label: string
  saving_label: string
  status: string
  message: string
  groups_title: string
  groups_empty: string
  refresh_label: string
  edit_keys_label: string
  bot_title: string
  bot_hint: string
  bot_status: string
  bot_save_label: string
  bot_field: Field
  groups: GroupRow[]
}
export interface FormRow { id: number; name: string; status: string; selected: boolean; open_label: string }
export interface ZoneTarget { key: string; label: string; marked: boolean; color: string }
export interface ZoneBox { key: string; label: string; x: number; y: number; w: number; h: number; color: string }
export interface FormEditor {
  id: number
  name: string
  sample_url: string
  hint: string
  targets: ZoneTarget[]
  boxes: ZoneBox[]
  clear_label: string
  rename_label: string
  delete_label: string
}
export interface FormsScreen {
  title: string
  hint: string
  empty_text: string
  name_label: string
  file_label: string
  create_label: string
  saving_label: string
  message: string
  forms: FormRow[]
  editor: FormEditor | null
}

export interface MentionRow { role: string; date: string; group: string; author: string; text: string; link: string }
export interface PersonCard { id: number; name: string; fields: Field[]; mentions: MentionRow[] | null }
export interface PersonBrief { id: number; name: string; lines: string[] }
export interface CandidateRow { a: PersonBrief; b: PersonBrief; actions: Action[] }
export interface Option { id: number; label: string }
export interface UnlinkedRow { id: number; mention: MentionRow; name: string; options: Option[] | null; link_label: string }
export interface PeopleScreen {
  title: string
  tab: string
  tabs: NavItem[]
  query: string
  search_label: string
  empty_text: string
  message: string
  open_label: string
  rows: SourceRow[] | null
  card: PersonCard | null
  candidates: CandidateRow[] | null
  unlinked: UnlinkedRow[] | null
}

export interface TableColumn { name: string; type: string; hint: string }
export interface TableScreen {
  title: string
  hint: string
  columns: TableColumn[]
  new_column: TableColumn
  types: { id: string; label: string }[]
  name_label: string
  type_label: string
  hint_label: string
  add_label: string
  remove_label: string
  include_queries: boolean
  include_label: string
  save_label: string
  saving_label: string
  message: string
  error: string
}
