package skillsbuiltin

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/OctoSucker/agent/pkg/llmclient"
	execbuiltin "github.com/OctoSucker/agent/repo/capability/builtin/exec"
)

// skillMarkdownParseSystemPrompt is the full system message for LLM skill extraction.
const skillMarkdownParseSystemPrompt = `You extract one skill definition from a markdown document.
Return only one JSON object with this exact shape (omit "source_path"; it is filled in by the host):
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
- "cautions" summarizes important warnings or constraints; use "" if none.
- "capabilities" groups tools by runtime. Prefer ONE group when every action uses the same subsystem (e.g. all shell commands → one "exec" group). Add another group only when the markdown clearly mixes distinct runtimes (e.g. exec and telegram).
- Do not invent capability names (e.g. skill-doc, documentation). Use only names from the Capability catalog below, or a deployment-specific MCP server name when the doc is clearly about that server.
- Each capability entry requires "capability" and "tools" (empty "tools" only if the markdown truly has none for that capability; prefer non-empty when the doc describes invocations).
- Each tool requires "name"; "description", "usage", and "input_schema" are optional.
- "input_schema" is a JSON object of argument keys when known from the markdown; otherwise {}.

Exec capability (important — matches runtime MCP):
- The planner invokes capability "exec" with a single tool name "run_command" (fixed). Shell/CLI examples are NOT separate tool names: the executable goes in arguments.program and the rest in arguments.args when the plan runs.
- For capability "exec", emit exactly ONE tool: {"name": "run_command", ...}. Put CLI documentation in "description", "usage" (representative invocations as plain text, e.g. program opencli with args ["install", "pkg"]), and "input_schema" (e.g. remind keys program, args, work_dir). Do not use names like "opencli", "npm", or "git" as tool "name" under exec — those are values of "program", not MCP tool ids.
- If the markdown is only a command reference for one CLI, still use one "run_command" entry and summarize subcommands in "usage".

Capability catalog (per group, which subsystem executes the action):
- exec — Runnable shell/CLI commands from the doc. Model them under the single tool "run_command" as above (not one tool per binary name).
- skills — ONLY when the markdown explicitly instructs using built-in skills tools (list_skills, get_skill, reload_skills, get_skills_planner_appendix). Do not use for ordinary CLI examples in the body.
- catalog — When the markdown clearly refers to catalog tools (e.g. just_chat_using_llm).
- telegram — When the markdown clearly refers to Telegram messaging tools.
- capability_registry — When the markdown clearly refers to listing capabilities or the planner tool appendix.

CLI / command-reference skills: almost always one capabilities[] entry with capability "exec" and exactly one tool named "run_command". Do not split one CLI into a second group for "documentation" or similar invalid labels.
`

func validateSkill(sk Skill) error {
	if sk.Name == "" {
		return fmt.Errorf("skills builtin: skill name is required")
	}
	if sk.Description == "" {
		return fmt.Errorf("skills builtin: skill description is required")
	}
	if len(sk.Capabilities) == 0 {
		return fmt.Errorf("skills builtin: at least one capability entry is required")
	}
	for i, c := range sk.Capabilities {
		if strings.TrimSpace(c.Capability) == "" {
			return fmt.Errorf("skills builtin: capabilities[%d]: capability name is required", i)
		}
		capName := strings.TrimSpace(c.Capability)
		if strings.EqualFold(capName, execbuiltin.CapabilityName) {
			if len(c.Tools) != 1 {
				return fmt.Errorf("skills builtin: capabilities[%d] (exec): exactly one tool named %q required; put CLI names in usage/description (they are arguments.program at plan time)", i, execbuiltin.ToolName)
			}
			if strings.TrimSpace(c.Tools[0].Name) != execbuiltin.ToolName {
				return fmt.Errorf("skills builtin: capabilities[%d] (exec): tool name must be %q, got %q", i, execbuiltin.ToolName, strings.TrimSpace(c.Tools[0].Name))
			}
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
	user := "source_file: " + filepath.Base(sourcePath) + "\n\n" + strings.TrimSpace(markdown)
	var out Skill
	if err := llm.CompleteJSON(ctx, skillMarkdownParseSystemPrompt, user, &out); err != nil {
		return Skill{}, fmt.Errorf("llm parse markdown: %w", err)
	}
	for i := range out.Capabilities {
		out.Capabilities[i].Capability = strings.TrimSpace(out.Capabilities[i].Capability)
	}
	if err := validateSkill(out); err != nil {
		return Skill{}, err
	}
	out.SourcePath = sourcePath
	return out, nil
}
