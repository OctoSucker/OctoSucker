package httpserver

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

func (s *server) serveChat(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxChatBodyBytes)
	var req chatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, chatResponse{Error: err.Error()}, "/api/chat")
		return
	}
	if _, err := io.Copy(io.Discard, r.Body); err != nil {
		writeJSON(w, http.StatusBadRequest, chatResponse{Error: err.Error()}, "/api/chat")
		return
	}
	msg := strings.TrimSpace(req.Message)
	if msg == "" {
		writeJSON(w, http.StatusBadRequest, chatResponse{Error: "empty message"}, "/api/chat")
		return
	}
	msgs, err := s.opts.RunChat(r.Context(), msg)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, chatResponse{Error: err.Error()}, "/api/chat")
		return
	}
	writeJSON(w, http.StatusOK, chatResponse{Messages: msgs}, "/api/chat")
}
