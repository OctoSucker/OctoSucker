package httpserver

import (
	"fmt"
	"net/http"
)

const maxChatBodyBytes = 1 << 20 // 1 MiB

type server struct {
	opts Options
}

// Handler builds the admin mux. Caller supplies IndexHTML and RunChat; Graph is optional.
func Handler(opts Options) (http.Handler, error) {
	if len(opts.IndexHTML) == 0 {
		return nil, fmt.Errorf("httpserver: IndexHTML required")
	}
	if opts.RunChat == nil {
		return nil, fmt.Errorf("httpserver: RunChat required")
	}
	s := &server{opts: opts}
	mux := http.NewServeMux()
	mux.Handle("GET /{$}", http.HandlerFunc(s.serveRoot))
	mux.Handle("POST /api/chat", http.HandlerFunc(s.serveChat))
	mux.Handle("GET /api/graph", http.HandlerFunc(s.serveGraph))
	return mux, nil
}
