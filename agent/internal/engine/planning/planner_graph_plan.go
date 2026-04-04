package planning

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/OctoSucker/agent/pkg/ports"
	"github.com/OctoSucker/agent/repo/tools/mcp"
	"github.com/OctoSucker/agent/repo/graph"
	"github.com/google/uuid"
)

// buildGraphPlan uses routing-graph frontier to choose a concrete tool, then asks LLM to fill only that tool's arguments.
func (p *Planner) buildGraphPlan(ctx context.Context, taskID string, task *ports.Task, pl ports.PayloadUserInput) (*ports.Plan, error) {
	lastNodePtr := &graph.Node{}
	excludeNode := &graph.Node{}
	if task.Plan != nil && len(task.Plan.Steps) > 0 {
		lastStep := task.Plan.Steps[len(task.Plan.Steps)-1]
		lastNodePtr = &lastStep.Node
		if lastStep.ToolResult.Err != nil {
			excludeNode = &lastStep.Node
		}
	}
	candidateNodes, err := p.RouteGraph.Frontier(ctx, task.UserInput, lastNodePtr, excludeNode, graph.FrontierSortAuto)
	if err != nil {
		return nil, fmt.Errorf("planner: graph candidateNodes: %w", err)
	}
	if len(candidateNodes) == 0 {
		return nil, fmt.Errorf("planner: graph candidateNodes is empty for task %s", taskID)
	}
	selectedNode := candidateNodes[0]

	systemPrompt, userPrompt, err := p.buildGraphPlanSystemPrompt(selectedNode, task.UserInput)
	if err != nil {
		return nil, fmt.Errorf("planner: graph plan system prompt: %w", err)
	}

	args := make(map[string]any)
	if err := p.PlannerLLM.CompleteJSON(ctx, systemPrompt, userPrompt, &args); err != nil {
		return nil, fmt.Errorf("planner: graph plan arguments json: %w", err)
	}
	toolSpec, err := p.RouteGraph.Tool(selectedNode.Tool)
	if err != nil {
		return nil, fmt.Errorf("planner: graph plan tool spec: %w", err)
	}
	if err := mcp.ValidateToolArguments(selectedNode.Tool, args, toolSpec.InputSchema); err != nil {
		return nil, fmt.Errorf("planner: validate tool arguments: %w", err)
	}
	parsed := &ports.PlanStep{
		ID:        uuid.New().String(),
		Goal:      task.UserInput,
		Node:      selectedNode,
		Arguments: args,
		Status:    "pending",
	}

	return &ports.Plan{Steps: []*ports.PlanStep{parsed}}, nil
}

func (p *Planner) buildGraphPlanSystemPrompt(selectedNode graph.Node, userInput string) (string, string, error) {

	systemPrompt := `
You are a tool argument generator for an AI agent.

Your job is to generate a valid JSON object for tool arguments.

You are given:
- a user task
- a specific tool name (flat id from the planner tool list)
- a JSON schema describing the tool input

You must generate arguments that strictly follow the schema.

You are NOT chatting.
You are NOT planning.
You ONLY generate arguments.

--------------------------------------------------
STRICT RULES

1. Output MUST be a valid JSON object
2. Do NOT output anything except JSON
3. Do NOT include markdown
4. Do NOT include explanations
5. All required fields in schema MUST be present
6. Field types MUST match the schema
7. Do NOT invent fields not in schema
8. If no arguments are needed, return {}

--------------------------------------------------
SELF CHECK BEFORE OUTPUT

- Is JSON valid?
- Does it match the schema?
- Are all required fields present?
- Are types correct?

Then output JSON only.

`

	toolSpec, err := p.RouteGraph.Tool(selectedNode.Tool)
	if err != nil {
		return "", "", fmt.Errorf("planner: graph plan tool spec: %w", err)
	}
	schemaRaw, err := json.Marshal(toolSpec.InputSchema)
	if err != nil {
		return "", "", fmt.Errorf("planner: marshal tool input schema: %w", err)
	}

	userPrompt := fmt.Sprintf(`
	[TASK]
	%s
	
	----------------------------------------
	[TOOL]
	%s
	
	----------------------------------------
	[TOOL INPUT SCHEMA]
	%s
	
	Generate arguments for this tool.
	
	Return ONLY a JSON object.
	`, userInput, selectedNode.Tool, string(schemaRaw))

	return systemPrompt, userPrompt, nil
}
