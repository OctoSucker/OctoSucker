package utils

import (
	"encoding/json"
	"net/http"
	"strings"
)

// HTTPNormalizeChatSessionID prefixes non-http- session ids for HTTP channel separation.
func HTTPNormalizeChatSessionID(s string) string {
	if strings.HasPrefix(s, "http-") {
		return s
	}
	return "http-" + s
}

// WriteJSON sets Content-Type application/json, status, and encodes v.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
