package memory

import (
	"context"
	"strings"
)

func BuildSummaryForQuery(ctx context.Context, m VectorMemory, query string, topK int,
	logf func(format string, args ...interface{})) string {
	if m == nil {
		return ""
	}
	items, err := m.Search(ctx, query, topK)
	if err != nil {
		if logf != nil {
			logf("[agent] memory search failed: %v", err)
		}
		return ""
	}
	if len(items) == 0 {
		return ""
	}
	var b strings.Builder
	for i, it := range items {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString("- ")
		b.WriteString(it.Text)
	}
	return b.String()
}
