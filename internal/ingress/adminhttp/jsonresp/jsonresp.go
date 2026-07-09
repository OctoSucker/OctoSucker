// Package jsonresp writes JSON HTTP responses for adminhttp route handlers.
package jsonresp

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
)

// Write encodes v as JSON with escaped HTML and sets Content-Type. route is used only for error logs.
func Write(w http.ResponseWriter, status int, v any, route string) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(true)
	if err := enc.Encode(v); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if status > 0 {
		w.WriteHeader(status)
	}
	if _, err := w.Write(buf.Bytes()); err != nil {
		log.Printf("adminhttp %s write: %v", route, err)
	}
}
