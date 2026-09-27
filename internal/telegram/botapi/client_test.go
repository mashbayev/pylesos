// Файл: internal/telegram/botapi/client_test.go
package botapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const token = "123456:ТЕСТОВЫЙ-токен"

func server(t *testing.T, handler func(w http.ResponseWriter, r *http.Request)) *Client {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(handler))
	t.Cleanup(ts.Close)
	return New(ts.URL, token, ts.Client())
}

func write(t *testing.T, w http.ResponseWriter, code int, body string) {
	t.Helper()
	w.WriteHeader(code)
	if _, err := w.Write([]byte(body)); err != nil {
		t.Errorf("ответ: %v", err)
	}
}

func TestGetMe(t *testing.T) {
	c := server(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/bot"+token+"/getMe" {
			write(t, w, 404, `{"ok":false,"error_code":404,"description":"Not Found"}`)
			return
		}
		write(t, w, 200, `{"ok":true,"result":{"id":42,"first_name":"Пылесос","username":"pylesos_test_bot"}}`)
	})
	me, err := c.GetMe(context.Background())
	if err != nil || me.ID != 42 || me.Username != "pylesos_test_bot" {
		t.Fatalf("%+v %v", me, err)
	}
}

func TestGetUpdatesParsesMessages(t *testing.T) {
	body := `{"ok":true,"result":[
	 {"update_id":10,"message":{"message_id":5,"date":1700000000,"chat":{"id":-1001234567890,"type":"supergroup","title":"Группа"},
	  "from":{"id":7,"first_name":"Иван","last_name":"Тестов"},"caption":"Подпись к фото",
	  "reply_to_message":{"message_id":3,"date":1,"chat":{"id":-1001234567890,"type":"supergroup"}},
	  "forward_origin":{"type":"hidden_user","sender_user_name":"Другой автопарк"},
	  "photo":[{"file_id":"small","file_size":100},{"file_id":"big","file_size":900}]}},
	 {"update_id":11,"edited_message":{"message_id":6,"date":1700000001,"edit_date":1700000100,
	  "chat":{"id":-1001234567890,"type":"supergroup"},"text":"Исправлено",
	  "document":{"file_id":"doc","file_name":"удостоверение.pdf","mime_type":"application/pdf","file_size":2048}}}]}`
	c := server(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("offset") != "10" || r.URL.Query().Get("timeout") != "30" ||
			!strings.Contains(r.URL.Query().Get("allowed_updates"), "edited_message") {
			write(t, w, 400, `{"ok":false,"error_code":400,"description":"bad query"}`)
			return
		}
		write(t, w, 200, body)
	})
	ups, err := c.GetUpdates(context.Background(), 10, 30)
	if err != nil || len(ups) != 2 {
		t.Fatalf("%+v %v", ups, err)
	}
	m := ups[0].Message
	if m.MessageID != 5 || m.Chat.ID != -1001234567890 || m.From.FirstName != "Иван" || m.Caption != "Подпись к фото" ||
		m.ReplyTo.MessageID != 3 || m.ForwardOrigin.SenderUserName != "Другой автопарк" || len(m.Photo) != 2 || len(m.Raw) == 0 {
		t.Fatalf("сообщение: %+v", m)
	}
	e := ups[1].EditedMessage
	if e.EditDate != 1700000100 || e.Text != "Исправлено" || e.Document.FileName != "удостоверение.pdf" {
		t.Fatalf("исправленное: %+v", e)
	}
}

func TestErrors(t *testing.T) {
	cases := map[int]error{401: ErrUnauthorized, 409: ErrConflict}
	for code, want := range cases {
		c := server(t, func(w http.ResponseWriter, _ *http.Request) {
			write(t, w, code, `{"ok":false,"error_code":`+http.StatusText(0)+`0,"description":"x"}`)
		})
		if _, err := c.GetUpdates(context.Background(), 0, 0); !errors.Is(err, want) {
			t.Fatalf("%d: %v", code, err)
		}
	}
	c := server(t, func(w http.ResponseWriter, _ *http.Request) {
		write(t, w, 500, `не JSON`)
	})
	_, err := c.GetMe(context.Background())
	if err == nil || strings.Contains(err.Error(), token) {
		t.Fatalf("ошибка без токена в тексте: %v", err)
	}
}
