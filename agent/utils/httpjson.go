package utils

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// HTTPNormalizeChatTaskID prefixes non-http- task ids for HTTP channel separation.
func HTTPNormalizeChatTaskID(s string) string {
	if strings.HasPrefix(s, "http-") {
		return s
	}
	return "http-" + s
}

// WriteJSON sets Content-Type application/json, status, and encodes v.
func WriteJSON(w http.ResponseWriter, status int, v any) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		return fmt.Errorf("write json: %w", err)
	}
	return nil
}
