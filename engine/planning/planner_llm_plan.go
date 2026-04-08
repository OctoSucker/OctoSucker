package planning

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"

	"github.com/OctoSucker/octosucker/engine/types"
	rt "github.com/OctoSucker/octosucker/repo/routegraph"
	"github.com/OctoSucker/octosucker/repo/toolprovider/mcp"
	"github.com/google/uuid"
)

// buildLLMPlan runs two LLM calls: (1) choose tool + goal with light planning rules, (2) fill arguments from that tool's schema only (+ prior steps as context).
func (p *Planner) buildLLMPlan(ctx context.Context, taskID string, task *types.Task) (*types.PlanStep, error) {
	prevSteps, err := task.Plan.FormatForPlannerPrompt()
	if err != nil {
		return nil, fmt.Errorf("planner: format prior steps: %w", err)
	}

	pickSys, pickUser, err := p.buildLLMPickStepPrompts(task, prevSteps)
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
		return nil, fmt.Errorf("planner: llm pick-step missing node.tool")
	}
	goal := strings.TrimSpace(pick.Goal)
	if goal == "" {
		goal = task.UserInput
	}

	toolSpec, err := p.ToolRegistry.Tool(toolID)
	if err != nil {
		return nil, fmt.Errorf("planner: tool: %w", err)
	}

	argSys, argUser, err := p.buildToolArgumentsPromptPair(task.UserInput, toolID, prevSteps)
	if err != nil {
		return nil, fmt.Errorf("planner: tool arguments prompt: %w", err)
	}
	args := make(map[string]any)
	if err := p.PlannerLLM.CompleteJSON(ctx, argSys, argUser, &args); err != nil {
		log.Printf("planner_llm: args JSON failed task=%s tool=%s err=%v", taskID, toolID, err)
		return nil, fmt.Errorf("planner: llm tool arguments json: %w", err)
	}

	if err := mcp.ValidateToolArguments(toolID, args, toolSpec.InputSchema); err != nil {
		log.Printf("planner_llm: args=%v schema=%v err=%v", args, toolSpec.InputSchema, err)
		return nil, fmt.Errorf("planner: validate tool arguments tool=%s schema=%v err=%w", toolID, toolSpec.InputSchema, err)
	}

	return &types.PlanStep{
		ID:        uuid.New().String(),
		Goal:      goal,
		Node:      rt.Node{Tool: toolID},
		Arguments: args,
		Status:    "pending",
	}, nil
}

func (p *Planner) buildLLMPickStepPrompts(task *types.Task, prevSteps string) (string, string, error) {
	const pickSystemPrompt = `
You are the planning module of an AI agent (step 1 of 2).

You only choose ONE next tool id and a short goal.
Do not output arguments. Do not execute tools. Do not chat.

Hard rules:
1) Always read [PREVIOUS STEPS] first.
2) If necessary and you don't have enough information from [PREVIOUS STEPS], read skills using tool "read_skill" and read tool lists using "list_tools_for_provider" to help you choose the next tool.
3) If choosing list_tools_for_provider, the provider must be a provider id from [AVAILABLE TOOL PROVIDERS].
4) Do not repeat list_tools_for_provider for a provider that already succeeded.
5) If a step failed with Tool error, do not pick the same failing action again without a concrete change.
6) When provider identity is uncertain, pick list_tool_providers first.

Output format:

Return JSON only, no markdown, no extra keys:

{
  "goal": "short outcome description (not the raw tool name)",
  "node": { "tool": "exact_tool_id" }
}
`

	toolProvidersAppendix := p.ToolRegistry.ProviderDescriptors()
	skillsAppendix := p.ToolRegistry.SkillsProvider.AllSkills()
	traj := strings.TrimSpace(task.TrajectorySummary)
	if traj == "" {
		traj = "(none)"
	}
	userPrompt := fmt.Sprintf(`
	[USER GOAL]
	%s

	----------------------------------------
	[AVAILABLE TOOL PROVIDERS — for tool "list_tools_for_provider"]
	%s

	----------------------------------------
	[AVAILABLE skills — for tool "read_skill"]
	%v

	----------------------------------------
	[PREVIOUS STEPS]
	%s

	----------------------------------------
	[TRAJECTORY JUDGE NOTE]
	%s

	Pick exactly one next tool id and goal. Do not include arguments.
	`, task.UserInput, toolProvidersAppendix, skillsAppendix, prevSteps, traj)

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

func clipPlannerGoal(s string) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) <= 120 {
		return s
	}
	return string(r[:120]) + "…"
}
