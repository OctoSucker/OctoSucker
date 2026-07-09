package planning

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"

	types "github.com/OctoSucker/octosucker/internal/runtime/model"
	rt "github.com/OctoSucker/octosucker/internal/runtime/toolrouting"
)

// selectLLMStep runs two LLM calls: (1) choose tool + goal with light planning rules, (2) fill arguments from that tool's schema only (+ prior steps as context).
func (p *Planner) selectLLMStep(ctx context.Context, taskID string, task *types.Task) (*types.PlanStep, error) {
	prevSteps, err := task.Plan.FormatForPlannerPrompt()
	if err != nil {
		return nil, fmt.Errorf("planner: format prior steps: %w", err)
	}

	var feedback string
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		pickSys, pickUser, err := p.buildLLMPickStepPrompts(task, prevSteps, feedback)
		if err != nil {
			return nil, fmt.Errorf("planner: pick-step prompt: %w", err)
		}

		var pick llmPickStepResponse
		if err := p.PlannerLLM.CompleteJSON(ctx, pickSys, pickUser, &pick); err != nil {
			log.Printf("planner_llm: pick-step JSON failed task=%s err=%v", taskID, err)
			return nil, fmt.Errorf("planner: llm pick-step json: %w", err)
		}
		toolID := strings.TrimSpace(pick.Node.Tool)
		if toolID == "" {
			lastErr = fmt.Errorf("planner: llm pick-step missing node.tool")
			feedback = lastErr.Error()
			continue
		}
		if toolID == "list_tools_for_provider" {
			_, mentioned := providerMentionedInText(task.UserInput+"\n"+prevSteps, p.providerIDs())
			_, inferred := providerForIntent(task.UserInput, p.providerIDs())
			if !mentioned && !inferred {
				toolID = "list_tool_providers"
			}
		}
		if _, err := p.ToolRegistry.Tool(toolID); err != nil {
			lastErr = fmt.Errorf("planner: picked unavailable tool %q: %w", toolID, err)
			feedback = lastErr.Error()
			continue
		}
		goal := strings.TrimSpace(pick.Goal)
		if goal == "" {
			goal = task.UserInput
		}

		args, err := p.buildArgumentsForTool(ctx, task.UserInput, goal, toolID, prevSteps, task.EvidenceSummary)
		if err != nil {
			lastErr = fmt.Errorf("planner: llm tool arguments: %w", err)
			feedback = lastErr.Error()
			continue
		}
		if repeatedFailedAction(toolID, args, task.EvidenceSummary) {
			lastErr = fmt.Errorf("planner: repeated failed action tool=%q args=%s", toolID, compactJSON(args))
			feedback = lastErr.Error()
			continue
		}
		if err := p.validateArgumentsForTool(toolID, args); err != nil {
			lastErr = err
			feedback = err.Error()
			continue
		}

		return newPendingPlanStep(goal, rt.Node{Tool: toolID}, args), nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("planner: llm failed to select a valid tool")
	}
	return nil, lastErr
}

func repeatedFailedAction(toolID string, args map[string]any, evidence string) bool {
	evidence = strings.TrimSpace(evidence)
	if toolID == "" || evidence == "" {
		return false
	}
	key := `- ` + toolID + ` `
	argText := `args=` + compactJSON(args)
	return strings.Contains(evidence, key) && strings.Contains(evidence, argText) && strings.Contains(evidence, " failed:")
}

func compactJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(b)
}

func (p *Planner) buildLLMPickStepPrompts(task *types.Task, prevSteps, feedback string) (string, string, error) {
	const pickSystemPrompt = `
You are the planning module of an AI agent (step 1 of 2).

You only choose ONE next tool id and a short goal.
Do not output arguments. Do not execute tools. Do not chat.

	Hard rules:
	1) Always read [PREVIOUS STEPS] first.
	2) For greetings, small talk, or simple direct questions that do not need external tools, choose "natural_language_reply" immediately.
	3) If necessary and you don't have enough information from [PREVIOUS STEPS], read skills using tool "read_skill" and read tool lists using "list_tools_for_provider" to help you choose the next tool.
	4) node.tool must be one exact value from [AVAILABLE TOOL IDS]. Tool provider ids and skill names are not tool ids.
	5) Do not repeat list_tools_for_provider for a provider that already succeeded.
	6) If a step failed with Tool error, do not pick the same failing action again without a concrete change.
	7) When provider identity is uncertain, pick list_tool_providers first.
	8) Discovery tools such as list_tool_providers, list_tools_for_provider, list_skills, and read_skill are only final answers when the user asked to inspect available capabilities. For real work, use their output as context and then pick the concrete execution tool in the next step.
	9) For multi-step tasks, pick the smallest useful next step that materially advances the original user goal. Do not summarize prior discovery as completion while there is still an executable next action.
	10) Respect [CURRENT TASK PHASE]:
	    - discovery: choose an information-gathering tool only if needed; otherwise move to the concrete execution tool.
	    - execution: choose a tool that performs the requested work using prior discovery context.
	    - synthesis: choose natural_language_reply only when no more tools are needed and the final answer can be written from prior outputs.
	11) Use read_skill only when the user asks to read documentation for an exact skill from [AVAILABLE SKILLS]. Do not treat website names, app names, provider ids, or targets such as "x", "twitter", or "login_x" as skill names.

Output format:

Return JSON only, no markdown, no extra keys:

{
  "goal": "short outcome description (not the raw tool name)",
  "node": { "tool": "exact_tool_id" }
}
`

	toolProvidersAppendix := plannerJSON(p.ToolRegistry.ProviderDescriptors())
	skillsAppendix := plannerJSON(p.ToolRegistry.SkillDescriptors())
	traj := strings.TrimSpace(task.TrajectorySummary)
	if traj == "" {
		traj = "(none)"
	}
	evidence := strings.TrimSpace(task.EvidenceSummary)
	if evidence == "" {
		evidence = "(none)"
	}
	project := strings.TrimSpace(p.ProjectCtx)
	if project == "" {
		project = "(none)"
	}
	feedback = strings.TrimSpace(feedback)
	if feedback == "" {
		feedback = "(none)"
	}
	userPrompt := fmt.Sprintf(`
	[USER GOAL]
	%s

	----------------------------------------
	[PROJECT INSTRUCTIONS]
	%s

	----------------------------------------
	[CURRENT TASK PHASE]
	%s

	----------------------------------------
	[AVAILABLE TOOL PROVIDERS — for tool "list_tools_for_provider"]
	%s

	----------------------------------------
	[AVAILABLE TOOL IDS — exact node.tool values]
	%s

	----------------------------------------
	[AVAILABLE SKILLS — exact names for tool "read_skill"]
	%v

	----------------------------------------
	[PREVIOUS STEPS]
	%s

	----------------------------------------
	[ACCUMULATED EVIDENCE]
	%s

	----------------------------------------
	[TRAJECTORY JUDGE NOTE]
	%s

	----------------------------------------
	[LAST PLANNER VALIDATION ERROR]
	%s

	Pick exactly one next tool id and goal. Do not include arguments.
	`, task.UserInput, project, task.Phase, toolProvidersAppendix, plannerJSON(p.ToolRegistry.AllToolIDs()), skillsAppendix, prevSteps, evidence, traj, feedback)

	return pickSystemPrompt, userPrompt, nil
}

type llmPickStepResponse struct {
	Goal string `json:"goal"`
	Node struct {
		Tool string `json:"tool"`
	} `json:"node"`
}

func sortedArgKeys(m map[string]any) []string {
	if len(m) == 0 {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func plannerJSON(v any) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(b)
}

func clipPlannerGoal(s string) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) <= 120 {
		return s
	}
	return string(r[:120]) + "…"
}
