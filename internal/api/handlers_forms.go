// Файл: internal/api/handlers_forms.go
package api

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	docdomain "pylesos/internal/docforms/domain"
	"pylesos/internal/tables"
	"pylesos/internal/texts"
)

// handleFormCreate — образец формы приходит в JSON как base64 (guard принимает только JSON).
func (s *Server) handleFormCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name     string `json:"name"`
		FileName string `json:"file_name"`
		Data     string `json:"data_base64"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, texts.ErrBadRequest)
		return
	}
	data, err := base64.StdEncoding.DecodeString(req.Data)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, texts.ErrBadRequest)
		return
	}
	screen, err := s.deps.Screens.FormsCreate(r.Context(), req.Name, req.FileName, data)
	s.writeScreen(w, "добавить форму", screen, err)
}

func (s *Server) handleFormSample(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, texts.ErrBadRequest)
		return
	}
	jpeg, err := s.deps.Samples.Sample(r.Context(), id)
	if errors.Is(err, docdomain.ErrNotFound) {
		s.writeError(w, http.StatusNotFound, texts.ErrFormsNotFound)
		return
	}
	if err != nil {
		s.deps.Log.Error("образец формы", "form", id, "err", err)
		s.writeError(w, http.StatusInternalServerError, texts.ErrInternal)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "no-store")
	if _, err := w.Write(jpeg); err != nil {
		s.deps.Log.Warn("отправить образец формы", "err", err)
	}
}

// formRequest — тело действий с формой: поле/рамка, зона (доли 0…1), новое название.
type formRequest struct {
	Key  string  `json:"key"`
	Name string  `json:"name"`
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
	W    float64 `json:"w"`
	H    float64 `json:"h"`
}

func (s *Server) handleFormAction(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, texts.ErrBadRequest)
		return
	}
	var req formRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, texts.ErrBadRequest)
		return
	}
	ctx := r.Context()
	switch r.PathValue("action") {
	case "zone":
		screen, err := s.deps.Screens.FormsSetZone(ctx, id, req.Key, tables.Zone{X: req.X, Y: req.Y, W: req.W, H: req.H})
		s.writeScreen(w, "отметка формы", screen, err)
	case "clear":
		screen, err := s.deps.Screens.FormsClearZone(ctx, id, req.Key)
		s.writeScreen(w, "снять отметку формы", screen, err)
	case "rename":
		screen, err := s.deps.Screens.FormsRename(ctx, id, req.Name)
		s.writeScreen(w, "переименовать форму", screen, err)
	case "delete":
		screen, err := s.deps.Screens.FormsDelete(ctx, id)
		s.writeScreen(w, "удалить форму", screen, err)
	default:
		s.writeError(w, http.StatusBadRequest, texts.ErrBadRequest)
	}
}
