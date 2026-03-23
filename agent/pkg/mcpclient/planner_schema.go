package mcpclient

import (
	"encoding/json"
	"strings"
)

func PlannerToolAppendix(tools []ToolSpec) string {
	if len(tools) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("MCP tools (names must match tool calls):\n")
	for _, t := range tools {
		b.WriteString("- ")
		b.WriteString(t.Name)
		if t.Description != "" {
			b.WriteString(": ")
			b.WriteString(t.Description)
		}
		if t.InputSchema != nil {
			raw, err := json.Marshal(t.InputSchema)
			if err == nil && len(raw) > 0 && string(raw) != "null" {
				b.WriteString(" params JSON Schema: ")
				b.Write(raw)
			}
		}
		b.WriteByte('\n')
	}
	return b.String()
}
