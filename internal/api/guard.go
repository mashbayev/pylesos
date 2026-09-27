// Файл: internal/api/guard.go
package api

import (
	"crypto/subtle"
	"net"
	"net/http"
	"strings"

	"pylesos/internal/texts"
)

// guard: Host только локальный (защита от DNS rebinding); для /api/* — токен и JSON в POST.
// Образец — doc-agent, internal/ui/server.go.
func (s *Server) guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, port, err := net.SplitHostPort(s.addr)
		if err != nil || (r.Host != s.addr && r.Host != "localhost:"+port) {
			s.writeError(w, http.StatusForbidden, texts.ErrForbidden)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			token := r.Header.Get(TokenHeader)
			if token == "" {
				token = r.URL.Query().Get("t")
			}
			if subtle.ConstantTimeCompare([]byte(token), []byte(s.token)) != 1 {
				s.writeError(w, http.StatusForbidden, texts.ErrForbidden)
				return
			}
			if r.Method == http.MethodPost && !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
				s.writeError(w, http.StatusUnsupportedMediaType, texts.ErrBadRequest)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
