// Файл: internal/telegram/domain/telegram.go
// Package domain — Telegram: группы, сообщения, вложения, ошибки входа.
package domain

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type PeerType string

const (
	PeerChannel PeerType = "channel" // супергруппа
	PeerChat    PeerType = "chat"    // обычная группа
)

// Peer — группа для API Telegram; JSON хранится в sources.config.
type Peer struct {
	Type       PeerType `json:"type"`
	ID         int64    `json:"id"`
	AccessHash int64    `json:"access_hash"`
}

// Key — ключ группы для экранов: «channel:ID:HASH».
func (p Peer) Key() string { return fmt.Sprintf("%s:%d:%d", p.Type, p.ID, p.AccessHash) }

var ErrBadPeerKey = errors.New("неверный ключ группы")

func ParsePeerKey(key string) (Peer, error) {
	parts := strings.Split(key, ":")
	if len(parts) != 3 || (parts[0] != string(PeerChannel) && parts[0] != string(PeerChat)) {
		return Peer{}, ErrBadPeerKey
	}
	id, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return Peer{}, ErrBadPeerKey
	}
	hash, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return Peer{}, ErrBadPeerKey
	}
	return Peer{Type: PeerType(parts[0]), ID: id, AccessHash: hash}, nil
}

type Group struct {
	Peer  Peer
	Title string
}

const (
	MediaPhoto    = "photo"
	MediaDocument = "document"
)

// Media — сведения о вложении; сам файл скачивается на разборе (П12).
type Media struct {
	Kind     string
	FileID   int64
	MimeType string
	FileName string
	Size     int64
}

type Message struct {
	ID         int
	Date       int64
	AuthorID   int64
	AuthorName string
	Text       string
	ReplyTo    int
	ThreadTop  int
	FwdFrom    string
	EditDate   int64
	Media      []Media
	Raw        string // сообщение Telegram в JSON
}

// Page — порция истории: обычные сообщения и границы номеров всего ответа Telegram,
// включая служебные («вступил в группу»). Lowest == 0 — ответ пуст, история кончилась.
type Page struct {
	Messages []Message
	Lowest   int
	Highest  int
}

// File — скачанное вложение; держится только в памяти (П12).
type File struct {
	Name string // имя с расширением — по нему выбирается формат страниц
	Data []byte
}

type Account struct {
	Authorized bool
	Name       string
}

var (
	ErrNotConfigured    = errors.New("не заданы api_id и api_hash")
	ErrNotAuthorized    = errors.New("вход в Telegram не выполнен")
	ErrPasswordNeeded   = errors.New("нужен облачный пароль")
	ErrBadPhone         = errors.New("Telegram не принял номер")
	ErrBadCode          = errors.New("неверный код")
	ErrCodeExpired      = errors.New("код устарел")
	ErrBadPassword      = errors.New("неверный облачный пароль")
	ErrNoAccount        = errors.New("на этот номер нет аккаунта Telegram")
	ErrBadKeys          = errors.New("Telegram не принял api_id или api_hash")
	ErrUnavailable      = errors.New("нет связи с Telegram")
	ErrGroupUnavailable = errors.New("нет доступа к группе")
)

// FloodWaitError — Telegram просит подождать (FLOOD_WAIT).
type FloodWaitError struct{ Wait time.Duration }

func (e *FloodWaitError) Error() string { return fmt.Sprintf("FLOOD_WAIT %s", e.Wait) }

// MessageLink — ссылка на сообщение супергруппы; у обычной группы ссылок нет — пусто.
func (p Peer) MessageLink(msgID int) string {
	if p.Type != PeerChannel {
		return ""
	}
	return fmt.Sprintf("https://t.me/c/%d/%d", p.ID, msgID)
}
