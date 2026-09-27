// Файл: internal/api/handlers.go
package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	jobsdomain "pylesos/internal/jobs/domain"
	jobsservice "pylesos/internal/jobs/service"
	"pylesos/internal/screens"
	"pylesos/internal/texts"
)

func (s *Server) handleScreen(w http.ResponseWriter, r *http.Request) {
	var (
		body any
		err  error
	)
	switch r.PathValue("name") {
	case "layout":
		body = s.deps.Screens.Layout()
	case "home":
		body, err = s.deps.Screens.Home(r.Context())
	case "jobs":
		body, err = s.deps.Screens.Jobs(r.Context())
	case "settings":
		body, err = s.deps.Screens.Settings(r.Context())
	case "telegram":
		body, err = s.deps.Screens.Telegram(r.Context())
	case "forms":
		body, err = s.deps.Screens.Forms(r.Context(), queryID(r))
	case "table":
		body, err = s.deps.Screens.Table(r.Context())
	case "people":
		q := r.URL.Query()
		body, err = s.deps.Screens.People(r.Context(), screens.PeopleQuery{Tab: q.Get("tab"), Query: q.Get("q"), Person: queryID(r)})
	default:
		s.writeError(w, http.StatusNotFound, texts.ErrBadRequest)
		return
	}
	if err != nil {
		s.deps.Log.Error("экран", "name", r.PathValue("name"), "err", err)
		s.writeError(w, http.StatusInternalServerError, texts.ErrInternal)
		return
	}
	s.writeJSON(w, http.StatusOK, body)
}

func (s *Server) handleSaveSettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Values map[string]string `json:"values"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, texts.ErrBadRequest)
		return
	}
	screen, err := s.deps.Screens.SaveSettings(r.Context(), req.Values)
	if err != nil {
		s.deps.Log.Error("сохранить настройки", "err", err)
		s.writeError(w, http.StatusInternalServerError, texts.ErrInternal)
		return
	}
	s.writeJSON(w, http.StatusOK, screen)
}

func (s *Server) handleJobAction(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, texts.ErrBadRequest)
		return
	}
	var act func() error
	switch jobsdomain.Action(r.PathValue("action")) {
	case jobsdomain.ActionPause:
		act = func() error { return s.deps.Jobs.Pause(r.Context(), id) }
	case jobsdomain.ActionResume:
		act = func() error { return s.deps.Jobs.Resume(r.Context(), id) }
	case jobsdomain.ActionCancel:
		act = func() error { return s.deps.Jobs.Cancel(r.Context(), id) }
	default:
		s.writeError(w, http.StatusBadRequest, texts.ErrBadRequest)
		return
	}
	if err := act(); err != nil {
		s.writeActionError(w, id, err)
		return
	}
	s.handleScreenJobs(w, r)
}

func (s *Server) handleScreenJobs(w http.ResponseWriter, r *http.Request) {
	screen, err := s.deps.Screens.Jobs(r.Context())
	if err != nil {
		s.deps.Log.Error("экран заданий", "err", err)
		s.writeError(w, http.StatusInternalServerError, texts.ErrInternal)
		return
	}
	s.writeJSON(w, http.StatusOK, screen)
}

func (s *Server) writeActionError(w http.ResponseWriter, id int64, err error) {
	switch {
	case errors.Is(err, jobsservice.ErrNotRunning), errors.Is(err, jobsservice.ErrNotAllowed):
		s.writeError(w, http.StatusConflict, texts.ErrActionUnavailable)
	case errors.Is(err, jobsdomain.ErrSourceBusy):
		s.writeError(w, http.StatusConflict, texts.ErrSourceBusy)
	case errors.Is(err, jobsdomain.ErrNotFound):
		s.writeError(w, http.StatusNotFound, texts.ErrJobNotFound)
	default:
		s.deps.Log.Error("действие с заданием", "job", id, "err", err)
		s.writeError(w, http.StatusInternalServerError, texts.ErrInternal)
	}
}

// handleShow — повторный запуск программы просит показать окно.
func (s *Server) handleShow(w http.ResponseWriter, _ *http.Request) {
	select {
	case s.show <- struct{}{}:
	default:
	}
	s.writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleQuit(w http.ResponseWriter, _ *http.Request) {
	s.writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	s.quitOnce.Do(func() { close(s.quit) })
}

// queryID — параметр ?id=; нет его или не число — 0 (ничего не выбрано).
func queryID(r *http.Request) int64 {
	id, err := strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
	if err != nil {
		return 0
	}
	return id
}
