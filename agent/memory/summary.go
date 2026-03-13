package memory

import (
	"context"
	"fmt"
	"strings"
)

const (
	PrefixInput  = "input:"
	PrefixTool   = "tool:"
	PrefixArgs   = "args:"
	PrefixResult = "result:"
	PrefixErr    = "err:"
	PrefixAnswer = "answer:"
)

func FormatToolStep(taskInput, toolName, args, result string, err error) string {
	return fmt.Sprintf("%s %s\n%s %s\n%s %s\n%s %s\n%s %v",
		PrefixInput, taskInput,
		PrefixTool, toolName,
		PrefixArgs, args,
		PrefixResult, result,
		PrefixErr, err)
}

func FormatTaskAnswer(taskInput, answer string) string {
	return fmt.Sprintf("%s %s\n%s %s", PrefixInput, taskInput, PrefixAnswer, answer)
}

func truncate(s string, max int) string {
	if max <= 0 {
		max = MaxMemoryTextLen
	}
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

// BuildSummaryForQuery 基于向量检索结果构建摘要，封装 VectorMemory.Search + BuildSummary。
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
