package types

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
