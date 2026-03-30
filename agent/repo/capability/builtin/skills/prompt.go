package skillsbuiltin

import (
	"encoding/json"
	"strings"
)

// PromptBundle is the skills slice plus filesystem root for planner prompts (Skills use the same shape as stored records; see Skill in store.go).
type PromptBundle struct {
	RootDir string
	Skills  []Skill
}

func FormatPromptAppendix(b PromptBundle) string {
	if len(b.Skills) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("Skills (from local markdown docs):\n")
	if b.RootDir != "" {
		sb.WriteString("Skills root directory: ")
		sb.WriteString(b.RootDir)
		sb.WriteByte('\n')
	}
	for _, sk := range b.Skills {
		sb.WriteString("- skill: ")
		sb.WriteString(sk.Name)
		if sk.Description != "" {
			sb.WriteString(" | ")
			sb.WriteString(sk.Description)
		}
		if sk.Cautions != "" {
			sb.WriteString(" | cautions: ")
			sb.WriteString(sk.Cautions)
		}
		sb.WriteString(" | source: ")
		sb.WriteString(sk.SourcePath)
		sb.WriteByte('\n')
		for _, c := range sk.Capabilities {
			sb.WriteString("  - capability: ")
			if strings.TrimSpace(c.Capability) != "" {
				sb.WriteString(c.Capability)
			} else {
				sb.WriteString("(unset)")
			}
			sb.WriteByte('\n')
			for _, t := range c.Tools {
				sb.WriteString("    - tool: ")
				sb.WriteString(t.Name)
				if t.Description != "" {
					sb.WriteString(" | ")
					sb.WriteString(t.Description)
				}
				if t.Usage != "" {
					sb.WriteString(" | usage: ")
					sb.WriteString(t.Usage)
				}
				if t.InputSchema != nil {
					raw, err := json.Marshal(t.InputSchema)
					if err != nil {
						return ""
					}
					sb.WriteString(" | input_schema: ")
					sb.Write(raw)
				}
				sb.WriteByte('\n')
			}
		}
	}
	return sb.String()
}
