package chat

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/OctoSucker/octosucker/internal/ingress/adminhttp/jsonresp"
)

const maxBodyBytes = 1 << 20 // 1 MiB

// Register adds POST /api/chat.
func Register(mux *http.ServeMux, run func(ctx context.Context, message string) ([]string, error)) {
	mux.Handle("POST /api/chat", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
		var req request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonresp.Write(w, http.StatusBadRequest, response{Error: err.Error()}, "/api/chat")
			return
		}
		if _, err := io.Copy(io.Discard, r.Body); err != nil {
			jsonresp.Write(w, http.StatusBadRequest, response{Error: err.Error()}, "/api/chat")
			return
		}
		msg := strings.TrimSpace(req.Message)
		if msg == "" {
			jsonresp.Write(w, http.StatusBadRequest, response{Error: "empty message"}, "/api/chat")
			return
		}
		msgs, err := run(r.Context(), msg)
		if err != nil {
			jsonresp.Write(w, http.StatusInternalServerError, response{Error: err.Error()}, "/api/chat")
			return
		}
		jsonresp.Write(w, http.StatusOK, response{Messages: msgs}, "/api/chat")
	}))
}

type request struct {
	Message string `json:"message"`
}

type response struct {
	Messages []string `json:"messages,omitempty"`
	Error    string   `json:"error,omitempty"`
}
