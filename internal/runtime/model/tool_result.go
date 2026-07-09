package model

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode"
)

// primaryTextMaxRunes is the default rune cap when feeding plan step text to an LLM.
const primaryTextMaxRunes = 16000

type ToolResult struct {
	Output any
	Err    error
	Kind   string
	Count  int
	Empty  bool
	Tool   string
}

type ToolPolicy struct {
	Capabilities []string `json:"capabilities,omitempty"`
	Risk         string   `json:"risk,omitempty"`
	Summary      string   `json:"summary,omitempty"`
}

func (res ToolResult) WithInferredMeta(tool string) ToolResult {
	res.Tool = strings.TrimSpace(tool)
	if res.Err != nil {
		res.Kind = "error"
		res.Empty = true
		res.Count = 0
		return res
	}
	if res.Kind == "" {
		res.Kind = inferOutputKind(res.Output)
	}
	if res.Count == 0 {
		res.Count = inferOutputCount(res.Output)
	}
	res.Empty = inferOutputEmpty(res.Output, res.Count)
	return res
}

// CompactForLLM returns Output squeezed for LLM context: structured values are walked
// so string leaves lose decorative CLI borders, then the result is JSON (indented when
// possible) or plain text, then rune-truncated for LLM context limits. Err is ignored.
// Nil Output yields "", nil.
func (res *ToolResult) CompactForLLM() string {
	if res == nil {
		return ""
	}
	if res.Err != nil {
		return res.Err.Error()
	}
	v := compactStructured(res.Output)
	if s, ok := v.(string); ok {
		return truncateRunes(s, primaryTextMaxRunes)
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		var errCompact error
		b, errCompact = json.Marshal(v)
		if errCompact != nil {
			return fmt.Sprintf("json marshal error: %v", res.Output)
		}
	}
	return truncateRunes(string(b), primaryTextMaxRunes)
}

func compactStructured(v any) any {
	switch x := v.(type) {
	case string:
		return compactDecorativeLines(x)
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, vv := range x {
			out[k] = compactStructured(vv)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i := range x {
			out[i] = compactStructured(x[i])
		}
		return out
	default:
		return v
	}
}

func compactDecorativeLines(s string) string {
	if s == "" {
		return s
	}
	var b strings.Builder
	skipping, firstOut := false, true
	for line := range strings.SplitSeq(s, "\n") {
		if isDecorativeLine(line) {
			if !skipping {
				if !firstOut {
					b.WriteByte('\n')
				}
				b.WriteString("…")
				firstOut = false
				skipping = true
			}
			continue
		}
		skipping = false
		if !firstOut {
			b.WriteByte('\n')
		}
		b.WriteString(line)
		firstOut = false
	}
	return b.String()
}

func isDecorativeLine(line string) bool {
	t := strings.TrimSpace(line)
	var runes, nonSpace, border int
	for _, r := range t {
		runes++
		if unicode.IsSpace(r) {
			continue
		}
		nonSpace++
		if isBorderRune(r) {
			border++
		}
	}
	if runes < 3 || nonSpace < 3 {
		return false
	}
	return border*100 >= 90*nonSpace
}

func isBorderRune(r rune) bool {
	if r >= 0x2500 && r <= 0x257f {
		return true
	}
	return strings.ContainsRune("-=*_+·│┃║", r)
}

func truncateRunes(s string, max int) string {
	if max <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}

func inferOutputKind(v any) string {
	switch x := v.(type) {
	case nil:
		return "empty"
	case string:
		return "text"
	case []any:
		return "list"
	case map[string]any:
		for _, key := range []string{"relations", "tools", "skills", "providers", "jobs"} {
			if _, ok := x[key]; ok {
				return key
			}
		}
		if _, ok := x["stdout"]; ok {
			return "command"
		}
		return "object"
	default:
		return "value"
	}
}

func inferOutputCount(v any) int {
	switch x := v.(type) {
	case nil:
		return 0
	case string:
		if strings.TrimSpace(x) == "" {
			return 0
		}
		return 1
	case []any:
		return len(x)
	case map[string]any:
		for _, key := range []string{"relations", "tools", "skills", "providers", "jobs"} {
			if arr, ok := x[key].([]any); ok {
				return len(arr)
			}
			if arr, ok := x[key].([]map[string]any); ok {
				return len(arr)
			}
			if arr, ok := x[key].([]string); ok {
				return len(arr)
			}
		}
		if stdout, ok := x["stdout"].(string); ok && strings.TrimSpace(stdout) != "" {
			return 1
		}
		return len(x)
	default:
		return 1
	}
}

func inferOutputEmpty(v any, count int) bool {
	if v == nil || count == 0 {
		return true
	}
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s) == ""
	}
	return false
}
