package httpserver

import "net/http"

func (s *server) serveRoot(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(s.opts.IndexHTML)
}
