package chat

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/OctoSucker/octosucker/internal/ingress/adminhttp/jsonresp"
	"github.com/OctoSucker/octosucker/internal/interaction"
)

const maxBodyBytes = 1 << 20 // 1 MiB

// Register adds POST /api/chat.
func Register(
	mux *http.ServeMux,
	run func(ctx context.Context, conversationID, message string) ([]string, error),
	plan func(ctx context.Context, messages []string) (*interaction.Interaction, error),
) {
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
		if msg == "" && len(req.InteractionResponse.Values) > 0 {
			msg = interaction.RenderResponse(req.InteractionResponse, nil)
		}
		if msg == "" {
			jsonresp.Write(w, http.StatusBadRequest, response{Error: "empty message"}, "/api/chat")
			return
		}
		conversationID := strings.TrimSpace(req.ConversationID)
		if conversationID == "" {
			conversationID = "admin"
		}
		msgs, err := run(r.Context(), conversationID, msg)
		if err != nil {
			jsonresp.Write(w, http.StatusInternalServerError, response{Error: err.Error()}, "/api/chat")
			return
		}
		var form *interaction.Interaction
		if plan != nil {
			form, _ = plan(r.Context(), msgs)
		}
		jsonresp.Write(w, http.StatusOK, response{
			ConversationID: conversationID,
			Messages:       msgs,
			Interaction:    form,
		}, "/api/chat")
	}))
}

type request struct {
	ConversationID      string               `json:"conversation_id,omitempty"`
	Message             string               `json:"message,omitempty"`
	InteractionResponse interaction.Response `json:"interaction_response,omitempty"`
}

type response struct {
	ConversationID string                   `json:"conversation_id,omitempty"`
	Messages       []string                 `json:"messages,omitempty"`
	Interaction    *interaction.Interaction `json:"interaction,omitempty"`
	Error          string                   `json:"error,omitempty"`
}
