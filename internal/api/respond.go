// Файл: internal/api/respond.go
package api

import (
	"encoding/json"
	"net/http"
)

func (s *Server) writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		s.deps.Log.Warn("отправить ответ", "err", err)
	}
}

// writeError отдаёт готовый текст для пользователя: {"error": "..."}.
func (s *Server) writeError(w http.ResponseWriter, code int, text string) {
	s.writeJSON(w, code, map[string]string{"error": text})
}
