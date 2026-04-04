package skillsbuiltin

import (
	"strconv"
	"strings"
)

// PromptBundle lists discovered skill files for planner prompts (content is loaded via read_skill).
type PromptBundle struct {
	RootDir string
	Skills  []SkillMeta
}

func (b *PromptBundle) FormatPromptAppendix() string {
	if len(b.Skills) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("Skills (markdown under skills root; load bodies with tool read_skill using pagination):\n")
	if b.RootDir != "" {
		sb.WriteString("Skills root directory: ")
		sb.WriteString(b.RootDir)
		sb.WriteByte('\n')
	}
	for _, sk := range b.Skills {
		sb.WriteString("- name: ")
		sb.WriteString(sk.Name)
		sb.WriteString(" | file: ")
		sb.WriteString(sk.SourceFile)
		sb.WriteString(" | bytes: ")
		sb.WriteString(strconv.FormatInt(sk.ByteSize, 10))
		sb.WriteByte('\n')
	}
	return sb.String()
}
