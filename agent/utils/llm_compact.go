package utils

import (
	"strings"
	"unicode"
)

// PrimaryTextMaxRunes is the default rune cap when feeding plan step text to an LLM.
const PrimaryTextMaxRunes = 16000

// CompactStructuredForLLM returns a deep copy of v with long decorative CLI/table
// borders collapsed inside string leaves (stdout tables, ruled separators, etc.).
func CompactStructuredForLLM(v any) any {
	switch x := v.(type) {
	case string:
		return CompactDecorativeLines(x)
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, vv := range x {
			out[k] = CompactStructuredForLLM(vv)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, vv := range x {
			out[i] = CompactStructuredForLLM(vv)
		}
		return out
	default:
		return v
	}
}

// CompactDecorativeLines collapses consecutive “border-only” lines into a single "…" line.
func CompactDecorativeLines(s string) string {
	if s == "" {
		return s
	}
	lines := strings.Split(s, "\n")
	var b strings.Builder
	skipping := false
	for _, line := range lines {
		if isDecorativeLine(line) {
			if !skipping {
				if b.Len() > 0 {
					b.WriteByte('\n')
				}
				b.WriteString("…")
				skipping = true
			}
			continue
		}
		skipping = false
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(line)
	}
	return b.String()
}

func isDecorativeLine(line string) bool {
	t := strings.TrimSpace(line)
	if len([]rune(t)) < 3 {
		return false
	}
	nonSpace := 0
	border := 0
	for _, r := range t {
		if unicode.IsSpace(r) {
			continue
		}
		nonSpace++
		if isBorderRune(r) {
			border++
		}
	}
	if nonSpace < 3 {
		return false
	}
	return border*100/nonSpace >= 90
}

func isBorderRune(r rune) bool {
	switch r {
	case '-', '=', '*', '_', '+', '·':
		return true
	case '│', '┃', '║':
		return true
	}
	return r >= 0x2500 && r <= 0x257f
}

// TruncateRunes returns s shortened to at most max runes, plus "…" when truncated.
func TruncateRunes(s string, max int) string {
	if max <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}
