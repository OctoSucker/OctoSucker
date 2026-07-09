package planning

import (
	"context"

	rt "github.com/OctoSucker/octosucker/internal/runtime/toolrouting"
	types "github.com/OctoSucker/octosucker/internal/runtime/model"
)

const graphRouteThreshold = 0.9

type planningRoute string

const (
	planningRouteDeterministic planningRoute = "deterministic"
	planningRouteGraph         planningRoute = "graph"
	planningRouteLLM           planningRoute = "llm"
)

func lastNodeForTask(task *types.Task) rt.Node {
	if task == nil || task.Plan == nil {
		return rt.Node{}
	}
	return task.Plan.LastNode()
}

func (p *Planner) graphRouteConfidence(ctx context.Context, userInput string, task *types.Task) float64 {
	return p.RouteGraph.Confidence(ctx, userInput, lastNodeForTask(task))
}

func (p *Planner) decidePlanningRoute(ctx context.Context, userInput string, task *types.Task) (planningRoute, float64) {
	if task != nil && task.Phase != "" && task.Phase != types.TaskPhaseExecution {
		return planningRouteLLM, 0
	}
	confidence := p.graphRouteConfidence(ctx, userInput, task)
	if confidence >= graphRouteThreshold {
		return planningRouteGraph, confidence
	}
	return planningRouteLLM, confidence
}
