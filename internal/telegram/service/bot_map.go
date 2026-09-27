// Файл: internal/telegram/service/bot_map.go
package service

import (
	"context"
	"encoding/json"
	"strings"

	sourcesdomain "pylesos/internal/sources/domain"
	"pylesos/internal/telegram/botapi"
	"pylesos/internal/telegram/domain"
)

// botChatID — chat_id супергруппы в Bot API: «-100» и id канала (03-telegram, грабли).
func botChatID(channelID int64) int64 { return -1_000_000_000_000 - channelID }

// channelIDOf — id канала из chat_id бота («-100…» → как у аккаунта), чтобы автор-канал
// записывался одинаково, кто бы ни принёс сообщение (ревью M2.1, №4). Прочие id — как есть.
func channelIDOf(chatID int64) int64 {
	if chatID < -1_000_000_000_000 {
		return -chatID - 1_000_000_000_000
	}
	return chatID
}

// mapUpdates раскладывает сообщения по источникам; чужие чаты, обычные группы и служебные сообщения — пропуск.
func (b *Bot) mapUpdates(ctx context.Context, ups []botapi.Update) (map[int64][]domain.Message, error) {
	chats, err := b.knownChats(ctx)
	if err != nil {
		return nil, err
	}
	out := map[int64][]domain.Message{}
	for _, u := range ups {
		m := u.Message
		if m == nil {
			m = u.EditedMessage
		}
		sourceID, ok := chats[chatIDOf(m)]
		if !ok || !meaningful(m) {
			continue
		}
		out[sourceID] = append(out[sourceID], fromBot(m))
	}
	return out, nil
}

func chatIDOf(m *botapi.Message) int64 {
	if m == nil {
		return 0
	}
	return m.Chat.ID
}

// knownChats — chat_id бота → источник, только для супергрупп.
func (b *Bot) knownChats(ctx context.Context) (map[int64]int64, error) {
	list, err := b.d.Sources.ListByKind(ctx, sourcesdomain.KindTelegramGroup)
	if err != nil {
		return nil, err
	}
	chats := map[int64]int64{}
	for _, s := range list {
		var p domain.Peer
		if json.Unmarshal([]byte(s.Config), &p) == nil && p.Type == domain.PeerChannel {
			chats[botChatID(p.ID)] = s.ID
		}
	}
	return chats, nil
}

// meaningful — есть текст, подпись, фото или документ (служебные «вступил в группу» — нет).
func meaningful(m *botapi.Message) bool {
	return m.Text != "" || m.Caption != "" || len(m.Photo) > 0 || m.Document != nil
}

func fromBot(m *botapi.Message) domain.Message {
	out := domain.Message{ID: m.MessageID, Date: m.Date, EditDate: m.EditDate, Text: m.Text, Raw: string(m.Raw)}
	if out.Text == "" {
		out.Text = m.Caption
	}
	out.AuthorID, out.AuthorName = botAuthor(m)
	if m.ReplyTo != nil {
		out.ReplyTo = m.ReplyTo.MessageID
	}
	if m.IsTopic {
		out.ThreadTop = m.MessageThreadID
	}
	out.FwdFrom = botForward(m.ForwardOrigin)
	out.Media = botMedia(m)
	return out
}

func botAuthor(m *botapi.Message) (int64, string) {
	if m.SenderChat != nil {
		return channelIDOf(m.SenderChat.ID), m.SenderChat.Title
	}
	if m.From != nil {
		return m.From.ID, botUserName(m.From)
	}
	return 0, ""
}

func botUserName(u *botapi.User) string {
	if name := strings.TrimSpace(u.FirstName + " " + u.LastName); name != "" {
		return name
	}
	if u.Username != "" {
		return "@" + u.Username
	}
	return ""
}

func botForward(o *botapi.ForwardOrigin) string {
	switch {
	case o == nil:
		return ""
	case o.SenderUserName != "":
		return o.SenderUserName
	case o.SenderUser != nil:
		return botUserName(o.SenderUser)
	case o.Chat != nil:
		return o.Chat.Title
	case o.SenderChat != nil:
		return o.SenderChat.Title
	}
	return ""
}

// botMedia — сведения о вложении; файл по ним не скачивается (П12) — на разборе его возьмёт аккаунт по номеру сообщения.
func botMedia(m *botapi.Message) []domain.Media {
	if len(m.Photo) > 0 {
		largest := m.Photo[len(m.Photo)-1]
		return []domain.Media{{Kind: domain.MediaPhoto, MimeType: "image/jpeg", Size: largest.FileSize}}
	}
	if m.Document != nil {
		return []domain.Media{{Kind: domain.MediaDocument, MimeType: m.Document.MimeType,
			FileName: m.Document.FileName, Size: m.Document.FileSize}}
	}
	return nil
}
