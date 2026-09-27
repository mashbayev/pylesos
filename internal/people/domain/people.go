// Файл: internal/people/domain/people.go
// Package domain — люди из Telegram-групп, их упоминания и кандидаты на склейку (П7, П17, П18).
package domain

import (
	"database/sql"
	"errors"
)

type Person struct {
	ID        int64          `db:"id"`
	FullName  string         `db:"full_name"`
	NameKey   string         `db:"name_key"`
	IIN       sql.NullString `db:"iin"`
	DocNumber string         `db:"doc_number"`
	BirthDate string         `db:"birth_date"`
	CreatedAt int64          `db:"created_at"`
}

// Role — чем является упоминание (спецификация, раздел 4, шаг 2).
type Role string

const (
	RoleRecord  Role = "record"  // «в ЧС, причина»
	RoleQuery   Role = "query"   // «есть инфо?»
	RoleComment Role = "comment" // реплика о человеке
)

// Link — как упоминание привязано к человеку.
type Link string

const (
	LinkIIN      Link = "iin"
	LinkName     Link = "name"
	LinkDocument Link = "document"
	LinkReply    Link = "reply"
	LinkManual   Link = "manual"
	LinkNone     Link = "none"
)

type Mention struct {
	ID            int64         `db:"id"`
	SourceID      int64         `db:"source_id"`
	MsgID         int           `db:"msg_id"`
	PersonID      sql.NullInt64 `db:"person_id"`
	Role          Role          `db:"role"`
	NameAsWritten string        `db:"name_as_written"`
	Link          Link          `db:"link"`
}

// Candidate — пара «возможно, один человек».
type Candidate struct {
	A int64 `db:"a"`
	B int64 `db:"b"`
}

var ErrNotFound = errors.New("человек не найден")

// Summary — человек со сводкой упоминаний (для списка на экране).
type Summary struct {
	Person
	Records  int    `db:"records"`
	Queries  int    `db:"queries"`
	Comments int    `db:"comments"`
	Groups   string `db:"groups"` // названия групп через запятую
}

// MentionView — упоминание с сообщением и группой (для карточки и «не привязано»).
type MentionView struct {
	Mention
	Date        int64  `db:"date"`
	Author      string `db:"author_name"`
	Text        string `db:"text"`
	SourceTitle string `db:"source_title"`
	SourceCfg   string `db:"source_config"`
}
