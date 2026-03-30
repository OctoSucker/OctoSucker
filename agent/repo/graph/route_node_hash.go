package graph

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// HashPipeJoinedRouteNodes returns first 8 hex chars of SHA256(nodes joined by "|").
func HashPipeJoinedRouteNodes(nodes []Node) string {
	parts := make([]string, len(nodes))
	for i := range nodes {
		parts[i] = nodes[i].String()
	}
	h := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return hex.EncodeToString(h[:])[:8]
}
