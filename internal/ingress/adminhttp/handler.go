package adminhttp

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/OctoSucker/octosucker/internal/ingress/adminhttp/chat"
	"github.com/OctoSucker/octosucker/internal/ingress/adminhttp/graph"
	"github.com/OctoSucker/octosucker/internal/ingress/adminhttp/root"
)

// Handler builds the admin mux. RunChat is required; IndexHTML defaults to the embedded shell; Graph is optional.
func Handler(opts Options) (http.Handler, error) {
	if opts.RunChat == nil {
		return nil, fmt.Errorf("adminhttp: RunChat required")
	}
	index := opts.IndexHTML
	if len(index) == 0 {
		index = embeddedShellHTML
	}
	if len(index) == 0 {
		return nil, fmt.Errorf("adminhttp: bundled shell missing")
	}
	mux := http.NewServeMux()
	root.Register(mux, index)
	chat.Register(mux, opts.RunChat, opts.PlanInteraction)
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
