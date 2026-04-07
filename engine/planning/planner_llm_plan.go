package planning

import (
	"context"
	"fmt"
	"log"

	"github.com/OctoSucker/octosucker/engine/types"
	"github.com/OctoSucker/octosucker/repo/toolprovider/mcp"
	"github.com/google/uuid"
)

func (p *Planner) buildLLMPlan(ctx context.Context, taskID string, task *types.Task, failureSummary string) (*types.Plan, error) {
	systemPrompt, userRequest, err := p.buildPlannerSystemPrompt(task, failureSummary)
	if err != nil {
		return nil, fmt.Errorf("planner: system prompt: %w", err)
	}
	log.Println("systemPrompt", systemPrompt)
	log.Println("userRequest", userRequest)

	var x llmPlanResponse
	if err := p.PlannerLLM.CompleteJSON(ctx, systemPrompt, userRequest, &x); err != nil {
		log.Printf("engine.Dispatcher: plan JSON parse failed task=%s err=%v", taskID, err)
		return nil, fmt.Errorf("planner: llm plan json: %w", err)
	}
	if len(x.Steps) == 0 {
		log.Printf("engine.Dispatcher: plan JSON had empty steps task=%s", taskID)
		return nil, fmt.Errorf("planner: llm returned no steps (at least one step is required)")
	}

	parsed := &types.Plan{}
	for _, st := range x.Steps {
		t, err := p.ToolRegistry.Tool(st.Node.Tool)
		if err != nil {
			return nil, fmt.Errorf("planner: tool: %w", err)
		}
		if err := mcp.ValidateToolArguments(st.Node.Tool, st.Arguments, t.InputSchema); err != nil {
			return nil, fmt.Errorf("planner: validate tool arguments: %w", err)
		}
		parsed.Steps = append(parsed.Steps, &types.PlanStep{
			ID:        uuid.New().String(),
			Goal:      st.Goal,
			Node:      st.Node,
			Arguments: st.Arguments,
			Status:    "pending",
		})
	}
	return parsed, nil
}

func (p *Planner) buildPlannerSystemPrompt(task *types.Task, failureSummary string) (string, string, error) {

	const systemPrompt = `
You are the planning module of an AI agent.

You ONLY generate a plan using tools.
You do NOT execute tools.
You do NOT chat.

--------------------------------------------------
PLAN DEFINITION

A plan is an ordered list of tool calls.

Each step:
- calls exactly one tool
- has a goal (outcome, not command)
- has arguments (valid JSON)

Steps must be minimal and complete.

--------------------------------------------------
PLANNING FLOW (MANDATORY)

For any task involving tools:

1) list_tools_for_provider (for relevant providers)
2) effect tool(s) to achieve the goal

Do NOT:
- skip discovery
- call effect tools before discovery
- repeat discovery steps

Exception:
If user only wants tool info → discovery only

--------------------------------------------------
TOOL RULES

Two tool types:

1) Introspection tools → return metadata
2) Effect tools → perform real actions

If user wants something DONE:
→ MUST include an effect tool

Never end with only introspection tools unless explicitly requested.

--------------------------------------------------
STEP RULES

Goal:
- describe outcome
- NOT tool name

Node:
{ "tool": "exact_tool_id" }

Arguments:
- must match schema
- use {} if empty

--------------------------------------------------
REPLANNING (IMPORTANT)

If LAST TOOL ERROR exists:

- Do NOT repeat failing step
- Fix arguments or tool choice
- Add introspection if needed

--------------------------------------------------
OUTPUT FORMAT (STRICT)

{
  "steps": [
    {
      "goal": "string",
      "node": { "tool": "tool_id" },
      "arguments": {}
    }
  ]
}

Rules:
- valid JSON only
- no extra text
- steps ≥ 1

--------------------------------------------------
CONSTRAINTS

- Max 8 steps (prefer 3–5)
- Use only AVAILABLE TOOLS
- Do not invent tool names

--------------------------------------------------
SELF CHECK

- starts with discovery
- includes effect tool if needed
- no repeated discovery
- valid JSON
`

	toolProvidersAppendix := p.ToolRegistry.ProviderDescriptors()
	skillsAppendix := p.ToolRegistry.SkillsProvider.AllSkills()
	userPrompt := fmt.Sprintf(`
	[USER GOAL]
	%s
	
	----------------------------------------
	[AVAILABLE TOOL PROVIDERS, use "list_tools_for_provider" tool to get tools for a provider]
	%s

	----------------------------------------
	[AVAILABLE skills PROVIDERS, use "read_skill" tool to get specific skill content]
	%v
	
	----------------------------------------
	[LAST TOOL ERROR]
	%s
	
	Generate a new plan.
	`, task.UserInput, toolProvidersAppendix, skillsAppendix, failureSummary)

	return systemPrompt, userPrompt, nil
}

type llmPlanResponse struct {
	Steps []types.PlanStep `json:"steps"`
}
