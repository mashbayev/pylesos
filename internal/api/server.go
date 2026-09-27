// Файл: internal/api/server.go
// Package api — HTTP для окна программы: экраны, действия, поток событий. Только 127.0.0.1 и токен запуска.
package api

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"sync"

	"pylesos/internal/events"
	"pylesos/internal/screens"
	"pylesos/internal/tables"
)

// TokenHeader — заголовок с токеном запуска (для SSE токен идёт параметром ?t=).
const TokenHeader = "X-Pylesos-Token"

type Screens interface {
	Layout() screens.Layout
	Home(ctx context.Context) (screens.HomeScreen, error)
	Jobs(ctx context.Context) (screens.JobsScreen, error)
	Settings(ctx context.Context) (screens.SettingsScreen, error)
	SaveSettings(ctx context.Context, input map[string]string) (screens.SettingsScreen, error)
	Telegram(ctx context.Context) (screens.TelegramScreen, error)
	TelegramSubmit(ctx context.Context, values map[string]string) (screens.TelegramScreen, error)
	TelegramAddGroup(ctx context.Context, key string) (screens.TelegramScreen, error)
	TelegramRefresh(ctx context.Context) (screens.TelegramScreen, error)
	TelegramEditKeys(ctx context.Context) (screens.TelegramScreen, error)
	TelegramBot(ctx context.Context, token string) (screens.TelegramScreen, error)
	Forms(ctx context.Context, selected int64) (screens.FormsScreen, error)
	FormsCreate(ctx context.Context, name, fileName string, data []byte) (screens.FormsScreen, error)
	FormsSetZone(ctx context.Context, id int64, key string, z tables.Zone) (screens.FormsScreen, error)
	FormsClearZone(ctx context.Context, id int64, key string) (screens.FormsScreen, error)
	People(ctx context.Context, q screens.PeopleQuery) (screens.PeopleScreen, error)
	PeopleMerge(ctx context.Context, a, b int64) (screens.PeopleScreen, error)
	PeopleReject(ctx context.Context, a, b int64) (screens.PeopleScreen, error)
	PeopleLink(ctx context.Context, mentionID, personID int64) (screens.PeopleScreen, error)
	Table(ctx context.Context) (screens.TableScreen, error)
	TableSave(ctx context.Context, cols []screens.TableColumn, includeQueries bool) (screens.TableScreen, error)
	FormsRename(ctx context.Context, id int64, name string) (screens.FormsScreen, error)
	FormsDelete(ctx context.Context, id int64) (screens.FormsScreen, error)
}

// FormSamples — образцы форм документов (картинка для разметки).
type FormSamples interface {
	Sample(ctx context.Context, id int64) ([]byte, error)
}

// SourceActions — кнопки у источника: «Собрать» и «Разобрать».
type SourceActions interface {
	Collect(ctx context.Context, sourceID int64) error
	Analyze(ctx context.Context, sourceID int64) error
	Export(ctx context.Context, sourceID int64) error
}

type JobActions interface {
	Pause(ctx context.Context, id int64) error
	Resume(ctx context.Context, id int64) error
	Cancel(ctx context.Context, id int64) error
}

type Deps struct {
	Token   string // пусто — сгенерировать
	Static  fs.FS
	Bus     *events.Bus
	Screens Screens
	Jobs    JobActions
	Sources SourceActions
	Samples FormSamples
	Log     *slog.Logger
}

type Server struct {
	deps     Deps
	token    string
	addr     string
	quit     chan struct{}
	quitOnce sync.Once
	show     chan struct{}
	stopping chan struct{} // закрывается StopStreams
	stopOnce sync.Once
}

func New(d Deps) (*Server, error) {
	token := d.Token
	if token == "" {
		buf := make([]byte, 32)
		if _, err := rand.Read(buf); err != nil {
			return nil, fmt.Errorf("токен запуска: %w", err)
		}
		token = base64.RawURLEncoding.EncodeToString(buf)
	}
	return &Server{deps: d, token: token, quit: make(chan struct{}), show: make(chan struct{}, 1),
		stopping: make(chan struct{})}, nil
}

// Listen занимает случайный порт на 127.0.0.1.
func (s *Server) Listen() (net.Listener, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("занять порт: %w", err)
	}
	s.addr = ln.Addr().String()
	return ln, nil
}

// URL — адрес окна программы с токеном запуска.
func (s *Server) URL() string { return "http://" + s.addr + "/?t=" + url.QueryEscape(s.token) }

func (s *Server) Quit() <-chan struct{} { return s.quit }
func (s *Server) Show() <-chan struct{} { return s.show }

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /", http.FileServerFS(s.deps.Static))
	mux.HandleFunc("GET /api/screens/{name}", s.handleScreen)
	mux.HandleFunc("POST /api/settings", s.handleSaveSettings)
	mux.HandleFunc("POST /api/jobs/{id}/{action}", s.handleJobAction)
	mux.HandleFunc("POST /api/telegram/submit", s.handleTelegramSubmit)
	mux.HandleFunc("POST /api/telegram/groups", s.handleTelegramGroup)
	mux.HandleFunc("POST /api/telegram/refresh", s.handleTelegramRefresh)
	mux.HandleFunc("POST /api/telegram/keys", s.handleTelegramKeys)
	mux.HandleFunc("POST /api/telegram/bot", s.handleTelegramBot)
	mux.HandleFunc("POST /api/sources/{id}/{action}", s.handleSourceAction)
	mux.HandleFunc("POST /api/people/{action}", s.handlePeopleAction)
	mux.HandleFunc("POST /api/table", s.handleTableSave)
	mux.HandleFunc("POST /api/forms", s.handleFormCreate)
	mux.HandleFunc("GET /api/forms/{id}/sample", s.handleFormSample)
	mux.HandleFunc("POST /api/forms/{id}/{action}", s.handleFormAction)
	mux.HandleFunc("GET /api/events", s.handleEvents)
	mux.HandleFunc("POST /api/show", s.handleShow)
	mux.HandleFunc("POST /api/quit", s.handleQuit)
	return s.guard(mux)
}
