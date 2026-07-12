// Package toolcontract defines the data exchanged between tool providers and
// the agent runtime. It deliberately depends on neither package.
package toolcontract

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"unicode"
)

const compactTextMaxRunes = 16000

// Result is the normalized result of one tool invocation.
type Result struct {
	Output    any
	Err       error
	Kind      string
	Count     int
	Empty     bool
	Tool      string
	Artifacts []ContextArtifact
}

const ArtifactAgentSkill = "agent_skill"

// ContextArtifact is trusted context produced by a tool that must remain
// available independently of the bounded execution trajectory.
type ContextArtifact struct {
	Kind    string `json:"kind"`
	Name    string `json:"name"`
	Content string `json:"content"`
	Source  string `json:"source,omitempty"`
	Digest  string `json:"digest,omitempty"`
	Trust   string `json:"trust"`
}

// Policy describes the capabilities and risk of one concrete tool call.
type Policy struct {
	Capabilities []string `json:"capabilities,omitempty"`
	Risk         string   `json:"risk,omitempty"`
	Summary      string   `json:"summary,omitempty"`
	OutputTrust  string   `json:"output_trust,omitempty"`
}

const (
	TrustUntrustedData        = "untrusted_data"
	TrustWorkspaceInstruction = "workspace_instruction"
	TrustRuntimeMetadata      = "runtime_metadata"
)

// ToolResult and ToolPolicy keep call sites explicit without coupling them to
// a concrete registry or runtime package.
type ToolResult = Result
type ToolPolicy = Policy

func (r Result) WithInferredMeta(tool string) Result {
	r.Tool = strings.TrimSpace(tool)
	if r.Err != nil {
		r.Kind = "error"
		r.Empty = true
		r.Count = 0
		return r
	}
	if r.Kind == "" {
		r.Kind = inferOutputKind(r.Output)
	}
	if r.Count == 0 {
		r.Count = inferOutputCount(r.Output)
	}
	r.Empty = inferOutputEmpty(r.Output, r.Count)
	return r
}

// CompactText renders structured output for bounded LLM context.
func (r Result) CompactText() string {
	if r.Err != nil {
		return truncateRunes(r.Err.Error(), compactTextMaxRunes)
	}
	v := compactStructured(r.Output)
	if s, ok := v.(string); ok {
		return truncateRunes(s, compactTextMaxRunes)
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		b, err = json.Marshal(v)
		if err != nil {
			return fmt.Sprintf("json marshal error: %v", r.Output)
		}
	}
	return truncateRunes(string(b), compactTextMaxRunes)
}

func compactStructured(v any) any {
	switch x := v.(type) {
	case string:
		return compactDecorativeLines(x)
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, value := range x {
			out[k] = compactStructured(value)
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
	skipping, first := false, true
	for line := range strings.SplitSeq(s, "\n") {
		if isDecorativeLine(line) {
			if !skipping {
				if !first {
					b.WriteByte('\n')
				}
				b.WriteString("...")
				first = false
				skipping = true
			}
			continue
		}
		skipping = false
		if !first {
			b.WriteByte('\n')
		}
		b.WriteString(line)
		first = false
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
		if (r >= 0x2500 && r <= 0x257f) || strings.ContainsRune("-=*_+|", r) {
			border++
		}
	}
	return runes >= 3 && nonSpace >= 3 && border*100 >= 90*nonSpace
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
			items, exists := x[key]
			if !exists {
				continue
			}
			switch items := items.(type) {
			case []any:
				return len(items)
			case []map[string]any:
				return len(items)
			case []string:
				return len(items)
			}
			value := reflect.ValueOf(items)
			if value.IsValid() && (value.Kind() == reflect.Slice || value.Kind() == reflect.Array) {
				return value.Len()
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

func truncateRunes(s string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "..."
}
