package adminhttp

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/OctoSucker/octosucker/internal/ingress/adminhttp/chat"
	"github.com/OctoSucker/octosucker/internal/ingress/adminhttp/graph"
	"github.com/OctoSucker/octosucker/internal/ingress/adminhttp/tasks"
)

// Handler builds the admin JSON API mux. RunChat is required; Graph is optional.
func Handler(opts Options) (http.Handler, error) {
	if opts.RunChat == nil {
		return nil, fmt.Errorf("adminhttp: RunChat required")
	}
	mux := http.NewServeMux()
	chat.Register(mux, opts.RunChat, opts.PlanInteraction)
	if opts.SubmitAssistantInput != nil && opts.SubmitTaskInteraction != nil && opts.SubmitTaskApproval != nil && opts.GetTask != nil {
		tasks.Register(mux, opts.SubmitAssistantInput, opts.SubmitTaskInteraction, opts.SubmitTaskApproval, opts.GetTask)
	}
	graph.Register(mux, opts.Graph)
	return localCORSMiddleware(mux), nil
}

func localCORSMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if isLocalOrigin(origin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.Header().Set("Access-Control-Allow-Methods", "GET,POST,OPTIONS")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func isLocalOrigin(origin string) bool {
	origin = strings.TrimSpace(origin)
	return strings.HasPrefix(origin, "http://127.0.0.1:") ||
		strings.HasPrefix(origin, "http://localhost:") ||
		strings.HasPrefix(origin, "http://[::1]:")
}
