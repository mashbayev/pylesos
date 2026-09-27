// Файл: internal/api/handlers_telegram.go
package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	jobsdomain "pylesos/internal/jobs/domain"
	"pylesos/internal/texts"
)

func (s *Server) handleTelegramSubmit(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Values map[string]string `json:"values"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, texts.ErrBadRequest)
		return
	}
	screen, err := s.deps.Screens.TelegramSubmit(r.Context(), req.Values)
	s.writeScreen(w, "вход в Telegram", screen, err)
}

func (s *Server) handleTelegramGroup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Key string `json:"key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, texts.ErrBadRequest)
		return
	}
	screen, err := s.deps.Screens.TelegramAddGroup(r.Context(), req.Key)
	s.writeScreen(w, "добавить группу", screen, err)
}

func (s *Server) handleTelegramRefresh(w http.ResponseWriter, r *http.Request) {
	screen, err := s.deps.Screens.TelegramRefresh(r.Context())
	s.writeScreen(w, "обновить группы", screen, err)
}

func (s *Server) handleTelegramKeys(w http.ResponseWriter, r *http.Request) {
	screen, err := s.deps.Screens.TelegramEditKeys(r.Context())
	s.writeScreen(w, "изменить ключи Telegram", screen, err)
}

func (s *Server) handleTelegramBot(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, texts.ErrBadRequest)
		return
	}
	screen, err := s.deps.Screens.TelegramBot(r.Context(), req.Token)
	s.writeScreen(w, "токен бота", screen, err)
}

// handleSourceAction — «Собрать» или «Разобрать»; в ответ — обновлённая главная.
func (s *Server) handleSourceAction(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, texts.ErrBadRequest)
		return
	}
	var start func(context.Context, int64) error
	switch r.PathValue("action") {
	case "collect":
		start = s.deps.Sources.Collect
	case "analyze":
		start = s.deps.Sources.Analyze
	case "export":
		start = s.deps.Sources.Export
	default:
		s.writeError(w, http.StatusNotFound, texts.ErrBadRequest)
		return
	}
	if err := start(r.Context(), id); err != nil {
		if errors.Is(err, jobsdomain.ErrSourceBusy) {
			s.writeError(w, http.StatusConflict, texts.ErrSourceBusy)
			return
		}
		s.deps.Log.Error("действие с источником", "source", id, "action", r.PathValue("action"), "err", err)
		s.writeError(w, http.StatusInternalServerError, texts.ErrInternal)
		return
	}
	screen, err := s.deps.Screens.Home(r.Context())
	s.writeScreen(w, "главная", screen, err)
}

// writeScreen отдаёт экран или логирует ошибку и отвечает текстом для пользователя.
func (s *Server) writeScreen(w http.ResponseWriter, what string, screen any, err error) {
	if err != nil {
		s.deps.Log.Error(what, "err", err)
		s.writeError(w, http.StatusInternalServerError, texts.ErrInternal)
		return
	}
	s.writeJSON(w, http.StatusOK, screen)
}
