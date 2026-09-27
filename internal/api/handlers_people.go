// Файл: internal/api/handlers_people.go
package api

import (
	"encoding/json"
	"net/http"

	"pylesos/internal/screens"
	"pylesos/internal/texts"
)

// handlePeopleAction — склеить / разные люди (пара a, b) или привязать упоминание к человеку (0 — новый).
func (s *Server) handlePeopleAction(w http.ResponseWriter, r *http.Request) {
	var req struct {
		A       int64 `json:"a"`
		B       int64 `json:"b"`
		Mention int64 `json:"mention"`
		Person  int64 `json:"person"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, texts.ErrBadRequest)
		return
	}
	var (
		screen screens.PeopleScreen
		err    error
	)
	switch r.PathValue("action") {
	case "merge":
		screen, err = s.deps.Screens.PeopleMerge(r.Context(), req.A, req.B)
	case "reject":
		screen, err = s.deps.Screens.PeopleReject(r.Context(), req.A, req.B)
	case "link":
		screen, err = s.deps.Screens.PeopleLink(r.Context(), req.Mention, req.Person)
	default:
		s.writeError(w, http.StatusNotFound, texts.ErrBadRequest)
		return
	}
	s.writeScreen(w, "люди", screen, err)
}

// handleTableSave — сохранить колонки таблицы; ошибка ввода приходит в поле error экрана.
func (s *Server) handleTableSave(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Columns        []screens.TableColumn `json:"columns"`
		IncludeQueries bool                  `json:"include_queries"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, texts.ErrBadRequest)
		return
	}
	screen, err := s.deps.Screens.TableSave(r.Context(), req.Columns, req.IncludeQueries)
	s.writeScreen(w, "таблица", screen, err)
}
