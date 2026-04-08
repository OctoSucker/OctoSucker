package planning

import (
	"context"
	"fmt"
	"log"

	"github.com/OctoSucker/octosucker/engine/types"
	rt "github.com/OctoSucker/octosucker/repo/routegraph"
	"github.com/OctoSucker/octosucker/repo/toolprovider/mcp"
	"github.com/google/uuid"
)

// buildGraphPlan uses routing-graph frontier to choose a concrete tool, then asks LLM to fill only that tool's arguments.
// It returns one pending PlanStep; HandleUserInput appends it to task.Plan.Steps.
func (p *Planner) buildGraphPlan(ctx context.Context, taskID string, task *types.Task) (*types.PlanStep, error) {
	lastNodePtr := &rt.Node{}
	excludeNode := &rt.Node{}
	if task.Plan != nil && len(task.Plan.Steps) > 0 {
		lastStep := task.Plan.Steps[len(task.Plan.Steps)-1]
		lastNodePtr = &lastStep.Node
		if lastStep.ToolResult.Err != nil {
			excludeNode = &lastStep.Node
		}
	}
	candidateNodes := p.RouteGraph.Frontier(ctx, task.UserInput, lastNodePtr, excludeNode)
	if len(candidateNodes) == 0 {
		return nil, fmt.Errorf("planner: graph candidateNodes is empty for task %s", taskID)
	}
	selectedNode := candidateNodes[0]

	systemPrompt, userPrompt, err := p.buildToolArgumentsPromptPair(task.UserInput, selectedNode.Tool, "")
	if err != nil {
		return nil, fmt.Errorf("planner: graph plan arguments prompt: %w", err)
	}

	args := make(map[string]any)
	if err := p.PlannerLLM.CompleteJSON(ctx, systemPrompt, userPrompt, &args); err != nil {
		return nil, fmt.Errorf("planner: graph plan arguments json: %w", err)
	}
	toolSpec, err := p.ToolRegistry.Tool(selectedNode.Tool)
	if err != nil {
		return nil, fmt.Errorf("planner: graph plan tool spec: %w", err)
	}
	if err := mcp.ValidateToolArguments(selectedNode.Tool, args, toolSpec.InputSchema); err != nil {
		log.Printf("args=%v schema=%v err=%v", args, toolSpec.InputSchema, err)
		return nil, fmt.Errorf("planner: validate tool arguments tool=%s schema=%v err=%w", selectedNode.Tool, toolSpec.InputSchema, err)
	}
	parsed := &types.PlanStep{
		ID:        uuid.New().String(),
		Goal:      task.UserInput,
		Node:      selectedNode,
		Arguments: args,
		Status:    "pending",
	}

	return parsed, nil
}
