package skillsbuiltin

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/OctoSucker/agent/pkg/llmclient"
)

type skillParseOutput struct {
	Name           string `json:"name"`
	Description    string `json:"description"`
	Cautions       string `json:"cautions"`
	Capabilities   []struct {
		Capability string      `json:"capability"`
		Tools      []SkillTool `json:"tools"`
	} `json:"capabilities"`
}

func validateSkillCapabilities(sk Skill) error {
	if len(sk.Capabilities) == 0 {
		return fmt.Errorf("skills builtin: at least one capability entry is required")
	}
	for i, c := range sk.Capabilities {
		if strings.TrimSpace(c.Capability) == "" {
			return fmt.Errorf("skills builtin: capabilities[%d]: capability name is required", i)
		}
		for j, t := range c.Tools {
			if strings.TrimSpace(t.Name) == "" {
				return fmt.Errorf("skills builtin: capabilities[%d].tools[%d]: name is required", i, j)
			}
		}
	}
	return nil
}

func parseSkillMarkdown(ctx context.Context, llm *llmclient.OpenAI, sourcePath string, markdown string) (Skill, error) {
	system := `You extract one skill definition from a markdown document.
Return only one JSON object with this exact shape:
{
  "name": "string",
  "description": "string",
  "cautions": "string",
  "capabilities": [
    {
      "capability": "string",
      "tools": [
        {
          "name": "string",
          "description": "string",
          "usage": "string",
          "input_schema": {}
        }
      ]
    }
  ]
}
Rules:
- "name" is required and concise.
- "description" is required and explains the skill purpose.
- "cautions" summarizes important warnings or constraints. Keep empty string if none.
- "capabilities" groups tools by which agent capability runs them (e.g. one entry for exec with shell-related tools, another for skills with skill-doc tools).
- Each capability entry has required "capability" (exactly one capability_name from the catalog appended below) and required "tools" array (may be empty only if the markdown truly has no tools for that capability—prefer non-empty tools when the doc describes invocations).
- Each tool has required "name", optional "description"/"usage"/"input_schema".
- "input_schema" should be a JSON object containing key args when clearly known from the markdown; otherwise use {}.
- Do not invent capabilities not present in the catalog below.`

	user := "source_file: " + filepath.Base(sourcePath) + "\n\n" + strings.TrimSpace(markdown)
	var out skillParseOutput
	if err := llm.CompleteJSON(ctx, system, user, &out); err != nil {
		return Skill{}, fmt.Errorf("llm parse markdown: %w", err)
	}
	if out.Name == "" {
		return Skill{}, fmt.Errorf("llm parse markdown: empty name")
	}
	if out.Description == "" {
		return Skill{}, fmt.Errorf("llm parse markdown: empty description")
	}
	caps := make([]SkillCapability, 0, len(out.Capabilities))
	for _, c := range out.Capabilities {
		caps = append(caps, SkillCapability{
			Capability: strings.TrimSpace(c.Capability),
			Tools:      c.Tools,
		})
	}
	sk := Skill{
		Name:         out.Name,
		Description:  out.Description,
		Cautions:     out.Cautions,
		SourcePath:   sourcePath,
		Capabilities: caps,
	}
	if err := validateSkillCapabilities(sk); err != nil {
		return Skill{}, err
	}
	return sk, nil
}
