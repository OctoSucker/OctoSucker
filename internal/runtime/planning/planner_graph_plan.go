package planning

import (
	"context"
	"fmt"

	types "github.com/OctoSucker/octosucker/internal/runtime/model"
	rt "github.com/OctoSucker/octosucker/internal/runtime/toolrouting"
)

// selectGraphStep uses routing-graph frontier to choose a concrete tool, then asks LLM to fill only that tool's arguments.
// It returns one pending PlanStep; HandleUserInput appends it to task.Plan.Steps.
func (p *Planner) selectGraphStep(ctx context.Context, taskID string, task *types.Task) (*types.PlanStep, error) {
	prevSteps, err := task.Plan.FormatForPlannerPrompt()
	if err != nil {
		return nil, fmt.Errorf("planner: format prior steps for graph route: %w", err)
	}
	lastNodePtr := &rt.Node{}
	excludeNode := &rt.Node{}
	if task.Plan != nil && task.Plan.HasSteps() {
		lastStep := task.Plan.LastStep()
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

	args, err := p.buildArgumentsForTool(ctx, task.UserInput, task.UserInput, selectedNode.Tool, prevSteps, task.EvidenceSummary)
	if err != nil {
		return nil, fmt.Errorf("planner: graph step arguments: %w", err)
	}
	if err := p.validateArgumentsForTool(selectedNode.Tool, args); err != nil {
		return nil, err
	}

	return newPendingPlanStep(task.UserInput, selectedNode, args), nil
}
