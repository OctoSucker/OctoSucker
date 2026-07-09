package root

import "net/http"

// Register adds GET / for the admin HTML shell.
func Register(mux *http.ServeMux, indexHTML []byte) {
	mux.Handle("GET /{$}", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(indexHTML)
	}))
}
