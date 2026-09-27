// Файл: internal/telegram/gotdgw/gotdgw_test.go
package gotdgw

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"

	"pylesos/internal/telegram/domain"
)

type memSecrets map[string][]byte

func (m memSecrets) Get(_ context.Context, key string) ([]byte, bool, error) {
	v, ok := m[key]
	return v, ok, nil
}

func (m memSecrets) Put(_ context.Context, key string, value []byte) error {
	m[key] = value
	return nil
}

func TestSessionStorage(t *testing.T) {
	ctx := context.Background()
	s := NewSessionStorage(memSecrets{})
	if _, err := s.LoadSession(ctx); !errors.Is(err, session.ErrNotFound) {
		t.Fatalf("нет сессии: %v", err)
	}
	if err := s.StoreSession(ctx, []byte("сессия")); err != nil {
		t.Fatal(err)
	}
	if got, err := s.LoadSession(ctx); err != nil || string(got) != "сессия" {
		t.Fatalf("got %q %v", got, err)
	}
}

func TestMapErr(t *testing.T) {
	cases := []struct {
		in   error
		want error
	}{
		{tgerr.New(400, "PHONE_CODE_INVALID"), domain.ErrBadCode},
		{tgerr.New(400, "PHONE_CODE_EXPIRED"), domain.ErrCodeExpired},
		{tgerr.New(400, "PHONE_NUMBER_INVALID"), domain.ErrBadPhone},
		{tgerr.New(400, "PASSWORD_HASH_INVALID"), domain.ErrBadPassword},
		{tgerr.New(401, "AUTH_KEY_UNREGISTERED"), domain.ErrNotAuthorized},
		{auth.ErrPasswordAuthNeeded, domain.ErrPasswordNeeded},
	}
	for _, c := range cases {
		if got := mapErr(c.in); !errors.Is(got, c.want) {
			t.Fatalf("%v → %v, ждали %v", c.in, got, c.want)
		}
	}
	var flood *domain.FloodWaitError
	if !errors.As(mapErr(tgerr.New(420, "FLOOD_WAIT_7")), &flood) || flood.Wait != 7*time.Second {
		t.Fatalf("FLOOD_WAIT: %+v", flood)
	}
	if mapErr(nil) != nil {
		t.Fatal("nil должен остаться nil")
	}
}

func TestMessagesFrom(t *testing.T) {
	m := &tg.Message{ID: 42, Date: 1700000000, Message: "Текст о водителе"}
	m.SetFromID(&tg.PeerUser{UserID: 5})
	reply := &tg.MessageReplyHeader{}
	reply.SetReplyToMsgID(40)
	m.SetReplyTo(reply)
	m.SetEditDate(1700000100)
	fwd := tg.MessageFwdHeader{}
	fwd.SetFromName("Другой автопарк")
	m.SetFwdFrom(fwd)
	photo := &tg.MessageMediaPhoto{}
	photo.SetPhoto(&tg.Photo{ID: 99})
	m.SetMedia(photo)
	user := &tg.User{ID: 5}
	user.SetFirstName("Иван")
	user.SetLastName("Тестов")
	res := &tg.MessagesMessages{
		Messages: []tg.MessageClass{m, &tg.MessageService{ID: 43}},
		Users:    []tg.UserClass{user},
	}
	page, err := messagesFrom(res)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Messages) != 1 {
		t.Fatalf("служебные сообщения не пропущены: %d", len(page.Messages))
	}
	if page.Lowest != 42 || page.Highest != 43 {
		t.Fatalf("границы порции должны учитывать служебные: %d..%d", page.Lowest, page.Highest)
	}
	got := page.Messages[0]
	if got.ID != 42 || got.AuthorID != 5 || got.AuthorName != "Иван Тестов" || got.ReplyTo != 40 ||
		got.EditDate != 1700000100 || got.FwdFrom != "Другой автопарк" || got.Text != "Текст о водителе" {
		t.Fatalf("разбор: %+v", got)
	}
	if len(got.Media) != 1 || got.Media[0].Kind != domain.MediaPhoto || got.Media[0].FileID != 99 || got.Raw == "" {
		t.Fatalf("вложение/raw: %+v", got)
	}
}

func TestDocumentAndAlive(t *testing.T) {
	doc := &tg.Document{ID: 7, MimeType: "application/pdf", Size: 2048,
		Attributes: []tg.DocumentAttributeClass{&tg.DocumentAttributeFilename{FileName: "удостоверение.pdf"}}}
	m := &tg.Message{ID: 1}
	media := &tg.MessageMediaDocument{}
	media.SetDocument(doc)
	m.SetMedia(media)
	page, err := messagesFrom(&tg.MessagesMessages{Messages: []tg.MessageClass{m}})
	msgs := page.Messages
	if err != nil || msgs[0].Media[0].FileName != "удостоверение.pdf" || msgs[0].Media[0].Kind != domain.MediaDocument {
		t.Fatalf("документ: %+v %v", msgs, err)
	}
	alive, err := aliveFrom(&tg.MessagesChannelMessages{Messages: []tg.MessageClass{&tg.Message{ID: 1}, &tg.MessageEmpty{ID: 2}}})
	if err != nil || !alive[1] || alive[2] {
		t.Fatalf("живые: %v %v", alive, err)
	}
}

func TestInputPeer(t *testing.T) {
	ch, ok := inputPeer(domain.Peer{Type: domain.PeerChannel, ID: 1, AccessHash: 2}).(*tg.InputPeerChannel)
	if !ok || ch.ChannelID != 1 || ch.AccessHash != 2 {
		t.Fatalf("канал: %+v", ch)
	}
	if _, ok := inputPeer(domain.Peer{Type: domain.PeerChat, ID: 3}).(*tg.InputPeerChat); !ok {
		t.Fatal("обычная группа")
	}
}

// Ошибки, которые gotd возвращает на самом деле (ревью M2: I1, I2, I7).
func TestMapErrRealGotdErrors(t *testing.T) {
	cases := []struct {
		in   error
		want error
	}{
		{auth.ErrPasswordInvalid, domain.ErrBadPassword},
		{&auth.SignUpRequired{}, domain.ErrNoAccount},
		{tgerr.New(400, "API_ID_INVALID"), domain.ErrBadKeys},
		{tgerr.New(400, "CONNECTION_API_ID_INVALID"), domain.ErrBadKeys},
		{tgerr.New(400, "CHANNEL_PRIVATE"), domain.ErrGroupUnavailable},
		{tgerr.New(403, "CHAT_FORBIDDEN"), domain.ErrGroupUnavailable},
	}
	for _, c := range cases {
		if got := mapErr(c.in); !errors.Is(got, c.want) {
			t.Fatalf("%v → %v, ждали %v", c.in, got, c.want)
		}
	}
}

// Порция из одних служебных сообщений: сообщений нет, но границы есть — история не кончилась (ревью M2, I4).
func TestServiceOnlyPageKeepsBounds(t *testing.T) {
	page, err := messagesFrom(&tg.MessagesMessages{Messages: []tg.MessageClass{&tg.MessageService{ID: 9}, &tg.MessageService{ID: 8}}})
	if err != nil || len(page.Messages) != 0 || page.Lowest != 8 || page.Highest != 9 {
		t.Fatalf("%+v %v", page, err)
	}
	empty, err := messagesFrom(&tg.MessagesMessages{})
	if err != nil || empty.Lowest != 0 {
		t.Fatalf("пустой ответ: %+v %v", empty, err)
	}
}

// M4: у фото берётся наибольший размер; у документа — имя файла или расширение по типу.
func TestLargestPhotoSizeAndDocName(t *testing.T) {
	photo := &tg.Photo{Sizes: []tg.PhotoSizeClass{
		&tg.PhotoSize{Type: "m", W: 320, H: 240},
		&tg.PhotoSizeProgressive{Type: "y", W: 1280, H: 960},
		&tg.PhotoSize{Type: "x", W: 800, H: 600},
	}}
	if got := largestSize(photo); got != "y" {
		t.Fatalf("наибольший размер %q", got)
	}
	named := &tg.Document{MimeType: "application/pdf", Attributes: []tg.DocumentAttributeClass{&tg.DocumentAttributeFilename{FileName: "права.pdf"}}}
	if got := docName(named); got != "права.pdf" {
		t.Fatalf("имя %q", got)
	}
	if got := docName(&tg.Document{MimeType: "image/png"}); got != "document.png" {
		t.Fatalf("без имени %q", got)
	}
}
