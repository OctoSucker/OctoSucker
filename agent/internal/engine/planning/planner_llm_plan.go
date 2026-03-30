package planning

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/OctoSucker/agent/pkg/ports"
	skillsbuiltin "github.com/OctoSucker/agent/repo/capability/builtin/skills"
	"github.com/OctoSucker/agent/repo/capability/mcp"
	"github.com/google/uuid"
)

func (p *Planner) buildLLMPlan(ctx context.Context, taskID string, task *ports.Task) (*ports.Plan, error) {
	baseUser, err := p.RecallCorpus.PlannerUserContent(ctx, task.UserInput.Text, 5)
	if err != nil {
		log.Printf("engine.Dispatcher: recall failed task=%s err=%v", taskID, err)
		return nil, fmt.Errorf("planner: recall: %w", err)
	}
	user := plannerLLMUserMessageWithReplanHint(task.PlannerReplanHint, baseUser)

	system, err := p.buildPlannerSystemPrompt()
	if err != nil {
		return nil, fmt.Errorf("planner: system prompt: %w", err)
	}
	var x llmPlanResponse
	if err := p.PlannerLLM.CompleteJSON(ctx, system, user, &x); err != nil {
		log.Printf("engine.Dispatcher: plan JSON parse failed task=%s err=%v", taskID, err)
		return nil, fmt.Errorf("planner: llm plan json: %w", err)
	}
	log.Printf("------planner: llm plan response: %+v", x)
	if len(x.Steps) == 0 {
		log.Printf("engine.Dispatcher: plan JSON had empty steps task=%s", taskID)
		return nil, fmt.Errorf("planner: llm returned no steps (at least one step is required)")
	}

	parsed := &ports.Plan{}
	for _, st := range x.Steps {
		t, err := p.RouteGraph.Tool(st.Capability, st.Tool)
		if err != nil {
			return nil, fmt.Errorf("planner: tool: %w", err)
		}
		if err := mcp.ValidateToolArguments(st.Tool, st.Arguments, t.InputSchema); err != nil {
			return nil, fmt.Errorf("planner: validate tool arguments: %w", err)
		}
		parsed.Steps = append(parsed.Steps, &ports.PlanStep{
			ID:         uuid.New().String(),
			Goal:       st.Goal,
			Capability: st.Capability,
			Tool:       st.Tool,
			Arguments:  st.Arguments,
			Status:     "pending",
		})
	}
	return parsed, nil
}

func (p *Planner) buildPlannerSystemPrompt() (string, error) {
	const planPreamble = `Reply with exactly one JSON object: {"steps":[...]}.
Each step needs "goal" (string). Set "capability" and "tool" to match one line in the tools appendix below (tool name and [capability=...] on that line). Include "arguments" per that line's JSON Schema, or {} / omit when empty.
"steps" must contain at least one step; run in array order. For greetings, small talk, or other purely conversational turns that need no filesystem/exec/external APIs, use capability=catalog and tool=just_chat_using_llm with the user's text in arguments.user_message; the engine already delivers the reply to the user—do not use Telegram or other send-message tools only to relay chat text.
When a capability lists multiple tools, set "tool" to the exact name from the appendix; if "tool" is omitted the runtime uses that capability's first tool only—prefer an explicit "tool" when schemas differ.
For exec, capability is always exec and tool is always run_command (see [capability=exec] in the appendix). Put the executable in arguments.program (e.g. opencli, npm, git). Use program sh or bash only when you need a shell; then arguments.args must be exactly two elements: "-c" and one string containing the full shell command (never pass bare sh with multiple tokens like ["npm","install"]—that is invalid).`

	toolApp, err := p.RouteGraph.PlannerToolAppendix()
	if err != nil {
		return "", err
	}
	system := planPreamble + "\n\n" + toolApp
	bundle, err := p.RouteGraph.PlannerSkills()
	if err != nil {
		return "", err
	}
	if sa := strings.TrimSpace(skillsbuiltin.FormatPromptAppendix(bundle)); sa != "" {
		system += "\n\n" + sa
		system += "\n\nWhen a skill matches the user intent, follow that skill's cautions and tool usage guidance while keeping plan steps executable with listed capabilities/tools. For capability=skills, the JSON \"tool\" field must match a tools-appendix line exactly (e.g. list_skills, reload_skills, get_skill, or a bound name like install-skill__run_command)—never the skill display title alone (e.g. not \"install-skill\")."
	}

	system += "\n\nWhen using exec for workspace files, set arguments.work_dir relative to workspace root and keep paths in arguments.args relative to work_dir; do not use host absolute paths like /Users/... or C:\\... in args."
	system += "\n\nExec work_dir is only allowed under the agent workspace roots (same tree as relative work_dir): never use /tmp, /var/tmp, or other paths outside that tree. For scratch space use a relative directory such as tmp or sandbox/tmp and mkdir -p it in an earlier sh step if needed."
	system += "\n\nIf the user message includes a section \"Recent tool failure (automatic replan)\", that block is only factual (failed goal, capability/tool, arguments JSON, error text). You must interpret it and choose the next steps—e.g. whether a missing binary requires prior install commands from the skills appendix. Do not repeat the same failing invocation unless the error is clearly transient (e.g. timeout)."
	system += "\n\nWhen you infer from the error text that arguments.program is unavailable (e.g. execvp() of 'PROGRAM' failed: No such file or directory, command not found, exit 127), treat PROGRAM as not installed or not on PATH in the exec environment—not bad flags. Do not emit a plan that is only another single step with the same program for the same user goal; add earlier exec steps that install PROGRAM (from the matching skill's install section or a standard package manager), then run PROGRAM again. If install commands are absent from skills, choose a reasonable package-manager exec step for the host OS."
	system += "\n\nSecondary fallback when the user only needs a normal browser URL opened on macOS and PROGRAM stays unavailable: use capability=exec, tool=run_command, program open, and pass the URL in arguments.args; if that also fails in replan, do not alternate forever—prefer telling the user via catalog/just_chat_using_llm that the host blocks GUI open from sandbox and they must install PROGRAM or run open outside the agent."
	system += "\n\nIgnore step id and status if you emit them: the engine assigns new ids and marks every new step pending."
	return system, nil
}

func plannerLLMUserMessageWithReplanHint(hint, baseUser string) string {
	h := strings.TrimSpace(hint)
	if h == "" {
		return baseUser
	}
	return "## Recent tool failure (automatic replan)\n" + h + "\n\n" + baseUser
}

type llmPlanResponse struct {
	Steps []ports.PlanStep `json:"steps"`
}
