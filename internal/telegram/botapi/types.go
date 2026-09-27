// Файл: internal/telegram/botapi/types.go
// Package botapi — минимальный клиент Telegram Bot API: getMe и getUpdates (long polling).
package botapi

import "encoding/json"

type User struct {
	ID        int64  `json:"id"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Username  string `json:"username"`
}

type Chat struct {
	ID    int64  `json:"id"`
	Type  string `json:"type"` // private, group, supergroup, channel
	Title string `json:"title"`
}

type MessageRef struct {
	MessageID int `json:"message_id"`
}

type ForwardOrigin struct {
	Type           string `json:"type"`
	SenderUser     *User  `json:"sender_user"`
	SenderUserName string `json:"sender_user_name"`
	Chat           *Chat  `json:"chat"`
	SenderChat     *Chat  `json:"sender_chat"`
}

type PhotoSize struct {
	FileID   string `json:"file_id"`
	FileSize int64  `json:"file_size"`
}

type Document struct {
	FileID   string `json:"file_id"`
	FileName string `json:"file_name"`
	MimeType string `json:"mime_type"`
	FileSize int64  `json:"file_size"`
}

type Message struct {
	MessageID       int             `json:"message_id"`
	Date            int64           `json:"date"`
	EditDate        int64           `json:"edit_date"`
	Chat            Chat            `json:"chat"`
	From            *User           `json:"from"`
	SenderChat      *Chat           `json:"sender_chat"`
	Text            string          `json:"text"`
	Caption         string          `json:"caption"`
	ReplyTo         *MessageRef     `json:"reply_to_message"`
	MessageThreadID int             `json:"message_thread_id"`
	IsTopic         bool            `json:"is_topic_message"`
	ForwardOrigin   *ForwardOrigin  `json:"forward_origin"`
	Photo           []PhotoSize     `json:"photo"`
	Document        *Document       `json:"document"`
	Raw             json.RawMessage `json:"-"` // сообщение как пришло
}

// UnmarshalJSON сохраняет исходный JSON сообщения в Raw.
func (m *Message) UnmarshalJSON(data []byte) error {
	type plain Message
	var p plain
	if err := json.Unmarshal(data, &p); err != nil {
		return err
	}
	*m = Message(p)
	m.Raw = append(json.RawMessage(nil), data...)
	return nil
}

type Update struct {
	UpdateID      int64    `json:"update_id"`
	Message       *Message `json:"message"`
	EditedMessage *Message `json:"edited_message"`
}
