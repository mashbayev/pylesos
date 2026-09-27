// Файл: internal/api/sse.go
package api

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// handleEvents — поток событий: окно перечитывает экраны по сигналу.
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	ch, unsubscribe := s.deps.Bus.Subscribe()
	defer unsubscribe()
	if _, err := fmt.Fprint(w, ": подключено\n\n"); err != nil {
		return
	}
	flusher.Flush()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-s.stopping:
			return
		case e, open := <-ch:
			if !open {
				return
			}
			data, err := json.Marshal(e)
			if err != nil {
				s.deps.Log.Error("событие в JSON", "err", err)
				continue
			}
			if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

// StopStreams закрывает потоки событий перед Shutdown: http.Server не отменяет контекст
// работающих запросов, и открытое окно держало бы выход до таймаута.
func (s *Server) StopStreams() { s.stopOnce.Do(func() { close(s.stopping) }) }
