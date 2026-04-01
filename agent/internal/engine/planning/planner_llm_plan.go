package planning

import (
	"context"
	"fmt"
	"log"

	"github.com/OctoSucker/agent/pkg/ports"
	"github.com/OctoSucker/agent/repo/capability/mcp"
	"github.com/google/uuid"
)

func (p *Planner) buildLLMPlan(ctx context.Context, taskID string, task *ports.Task, failureSummary string) (*ports.Plan, error) {

	systemPrompt, userRequest, err := p.buildPlannerSystemPrompt(ctx, task, failureSummary)
	if err != nil {
		return nil, fmt.Errorf("planner: system prompt: %w", err)
	}
	var x llmPlanResponse
	if err := p.PlannerLLM.CompleteJSON(ctx, systemPrompt, userRequest, &x); err != nil {
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
		t, err := p.RouteGraph.Tool(st.Node.Capability, st.Node.Tool)
		if err != nil {
			return nil, fmt.Errorf("planner: tool: %w", err)
		}
		if err := mcp.ValidateToolArguments(st.Node.Tool, st.Arguments, t.InputSchema); err != nil {
			return nil, fmt.Errorf("planner: validate tool arguments: %w", err)
		}
		parsed.Steps = append(parsed.Steps, &ports.PlanStep{
			ID:        uuid.New().String(),
			Goal:      st.Goal,
			Node:      st.Node,
			Arguments: st.Arguments,
			Status:    "pending",
		})
	}
	return parsed, nil
}

func (p *Planner) buildPlannerSystemPrompt(ctx context.Context, task *ports.Task, failureSummary string) (string, string, error) {

	const systemPrompt = `
You are the planning module of an AI agent.

Your job is to generate a plan to achieve the user's goal by calling capability tools.

You are NOT chatting.
You are NOT executing tools.
You ONLY generate a plan.

--------------------------------------------------
PLANNING RULES

1. The plan must achieve the user's goal
2. Steps are executed in order
3. Each step must call exactly one tool
4. Use only provided capabilities and tools
5. Do NOT invent capabilities or tools
6. Keep the plan minimal but complete
7. goal describes the outcome, NOT the command
8. arguments must match the tool parameter schema

--------------------------------------------------
NODE FORMAT (VERY IMPORTANT)

Each step must contain a node object:
--------------------------------------------------
EXEC TOOL RULES

For exec capability:

- arguments.program must be the executable (git, npm, etc.)
- Use "sh" ONLY when necessary
- When using shell:
  arguments.args MUST be:
  ["-c", "full command string"]

--------------------------------------------------
OUTPUT FORMAT

Return exactly one JSON object:

{
  "steps": [
    {
      "goal": "string",
      "node": {
        "capability": "string",
        "tool": "string"
      },
      "arguments": {}
    }
  ]
}

Rules:
- MUST be valid JSON
- NO extra text outside JSON
- steps must contain at least one step
- arguments must be an object (use {} if empty)

--------------------------------------------------
SELF CHECK BEFORE OUTPUT

- JSON is valid
- Each step has goal, node, arguments
- node.capability exists
- node.tool exists
- arguments is an object
- The plan actually helps achieve the user's goal
`

	skillsAppendix, err := p.RouteGraph.PlannerSkills()
	if err != nil {
		return "", "", err
	}
	toolsAppendix, err := p.RouteGraph.PlannerToolAppendix()
	if err != nil {
		return "", "", err
	}
	userHistory, err := p.RecallCorpus.RecallUserHistory(ctx, task.UserInput, 5)
	if err != nil {
		log.Printf("engine.Dispatcher: recall failed err=%v", err)
		return "", "", fmt.Errorf("planner: recall: %w", err)
	}

	userPrompt := fmt.Sprintf(`
	[USER GOAL]
	%s
	
	----------------------------------------
	[AVAILABLE SKILLS]
	%s
	
	----------------------------------------
	[AVAILABLE CAPABILITIES AND TOOLS]
	%s
	
	----------------------------------------
	[USER HISTORY]
	%s
	
	----------------------------------------
	[LAST TOOL ERROR]
	%s
	
	Generate a new plan.
	`, task.UserInput, skillsAppendix, toolsAppendix, userHistory, failureSummary)

	return systemPrompt, userPrompt, nil
}

type llmPlanResponse struct {
	Steps []ports.PlanStep `json:"steps"`
}
