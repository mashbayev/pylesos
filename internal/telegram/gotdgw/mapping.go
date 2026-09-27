// Файл: internal/telegram/gotdgw/mapping.go
package gotdgw

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gotd/td/telegram/query/dialogs"
	"github.com/gotd/td/tg"

	"pylesos/internal/telegram/domain"
)

// messagesFrom разбирает ответ истории: в Messages — только обычные сообщения, служебные («вступил в группу»)
// пропускаются, но учитываются в границах порции Lowest/Highest (ревью M2, I4).
func messagesFrom(res tg.MessagesMessagesClass) (domain.Page, error) {
	mod, ok := res.AsModified()
	if !ok {
		return domain.Page{}, fmt.Errorf("неожиданный ответ истории %T", res)
	}
	users, chats := entitiesOf(mod)
	page := domain.Page{Messages: []domain.Message{}}
	for _, m := range mod.GetMessages() {
		if _, empty := m.(*tg.MessageEmpty); empty {
			continue
		}
		page.Lowest, page.Highest = bounds(page, m.GetID())
		if msg, ok := m.(*tg.Message); ok {
			dm, err := toMessage(msg, users, chats)
			if err != nil {
				return domain.Page{}, err
			}
			page.Messages = append(page.Messages, dm)
		}
	}
	return page, nil
}

func bounds(p domain.Page, id int) (int, int) {
	if p.Lowest == 0 {
		return id, id
	}
	return min(p.Lowest, id), max(p.Highest, id)
}

func entitiesOf(mod tg.ModifiedMessagesMessages) (map[int64]*tg.User, map[int64]string) {
	users := map[int64]*tg.User{}
	for _, u := range mod.GetUsers() {
		if user, ok := u.(*tg.User); ok {
			users[user.GetID()] = user
		}
	}
	chats := map[int64]string{}
	for _, c := range mod.GetChats() {
		switch ch := c.(type) {
		case *tg.Channel:
			chats[ch.GetID()] = ch.GetTitle()
		case *tg.Chat:
			chats[ch.GetID()] = ch.GetTitle()
		}
	}
	return users, chats
}

func toMessage(m *tg.Message, users map[int64]*tg.User, chats map[int64]string) (domain.Message, error) {
	raw, err := json.Marshal(m)
	if err != nil {
		return domain.Message{}, fmt.Errorf("сообщение %d в JSON: %w", m.GetID(), err)
	}
	out := domain.Message{ID: m.GetID(), Date: int64(m.GetDate()), Text: m.GetMessage(), Media: mediaOf(m), Raw: string(raw)}
	if from, ok := m.GetFromID(); ok {
		out.AuthorID, out.AuthorName = author(from, users, chats)
	}
	if edit, ok := m.GetEditDate(); ok {
		out.EditDate = int64(edit)
	}
	if reply, ok := m.GetReplyTo(); ok {
		if h, ok := reply.(*tg.MessageReplyHeader); ok {
			out.ReplyTo, _ = h.GetReplyToMsgID()
			out.ThreadTop, _ = h.GetReplyToTopID()
		}
	}
	if fwd, ok := m.GetFwdFrom(); ok {
		out.FwdFrom = forwardedFrom(fwd, users, chats)
	}
	return out, nil
}

func author(p tg.PeerClass, users map[int64]*tg.User, chats map[int64]string) (int64, string) {
	switch v := p.(type) {
	case *tg.PeerUser:
		return v.GetUserID(), userName(users[v.GetUserID()])
	case *tg.PeerChannel:
		return v.GetChannelID(), chats[v.GetChannelID()]
	case *tg.PeerChat:
		return v.GetChatID(), chats[v.GetChatID()]
	}
	return 0, ""
}

func userName(u *tg.User) string {
	if u == nil {
		return ""
	}
	first, _ := u.GetFirstName()
	last, _ := u.GetLastName()
	if name := strings.TrimSpace(first + " " + last); name != "" {
		return name
	}
	if nick, ok := u.GetUsername(); ok {
		return "@" + nick
	}
	return ""
}

func forwardedFrom(f tg.MessageFwdHeader, users map[int64]*tg.User, chats map[int64]string) string {
	if name, ok := f.GetFromName(); ok {
		return name
	}
	if from, ok := f.GetFromID(); ok {
		_, name := author(from, users, chats)
		return name
	}
	return ""
}

func mediaOf(m *tg.Message) []domain.Media {
	media, ok := m.GetMedia()
	if !ok {
		return nil
	}
	switch v := media.(type) {
	case *tg.MessageMediaPhoto:
		if p, ok := v.GetPhoto(); ok {
			if photo, ok := p.(*tg.Photo); ok {
				return []domain.Media{{Kind: domain.MediaPhoto, FileID: photo.GetID(), MimeType: "image/jpeg"}}
			}
		}
	case *tg.MessageMediaDocument:
		if d, ok := v.GetDocument(); ok {
			if doc, ok := d.(*tg.Document); ok {
				return []domain.Media{documentOf(doc)}
			}
		}
	}
	return nil
}

func documentOf(d *tg.Document) domain.Media {
	md := domain.Media{Kind: domain.MediaDocument, FileID: d.GetID(), MimeType: d.GetMimeType(), Size: d.GetSize()}
	for _, a := range d.GetAttributes() {
		if fn, ok := a.(*tg.DocumentAttributeFilename); ok {
			md.FileName = fn.GetFileName()
		}
	}
	return md
}

// groupOf — группа из элемента списка диалогов; каналы-рассылки и личные чаты пропускаются.
func groupOf(e dialogs.Elem) (domain.Group, bool) {
	switch p := e.Peer.(type) {
	case *tg.InputPeerChannel:
		ch, ok := e.Entities.Channel(p.ChannelID)
		if !ok || !ch.GetMegagroup() {
			return domain.Group{}, false
		}
		return domain.Group{Peer: domain.Peer{Type: domain.PeerChannel, ID: p.ChannelID, AccessHash: p.AccessHash}, Title: ch.GetTitle()}, true
	case *tg.InputPeerChat:
		c, ok := e.Entities.Chat(p.ChatID)
		if !ok {
			return domain.Group{}, false
		}
		return domain.Group{Peer: domain.Peer{Type: domain.PeerChat, ID: p.ChatID}, Title: c.GetTitle()}, true
	}
	return domain.Group{}, false
}

func inputPeer(p domain.Peer) tg.InputPeerClass {
	if p.Type == domain.PeerChat {
		return &tg.InputPeerChat{ChatID: p.ID}
	}
	return &tg.InputPeerChannel{ChannelID: p.ID, AccessHash: p.AccessHash}
}

// aliveFrom — номера сообщений, которые ещё существуют (удалённое приходит как MessageEmpty).
func aliveFrom(res tg.MessagesMessagesClass) (map[int]bool, error) {
	mod, ok := res.AsModified()
	if !ok {
		return nil, fmt.Errorf("неожиданный ответ %T", res)
	}
	alive := map[int]bool{}
	for _, m := range mod.GetMessages() {
		if _, empty := m.(*tg.MessageEmpty); !empty {
			alive[m.GetID()] = true
		}
	}
	return alive, nil
}
