package judge

import (
	"github.com/OctoSucker/agent/pkg/llmclient"
	routinggraph "github.com/OctoSucker/agent/repo/routing_graph"
	"github.com/OctoSucker/agent/repo/task"
)

// Judge owns post-execution evaluation and finalization handlers.
type Judge struct {
	StepCritic       *StepCritic
	TrajectoryCritic *TrajectoryCritic
}

func NewJudge(
	tasks *task.TaskStore,
	routeGraph *routinggraph.RoutingGraph,
	trajectoryLLM *llmclient.OpenAI,
) *Judge {
	return &Judge{
		StepCritic:       NewStepCritic(tasks, routeGraph),
		TrajectoryCritic: NewTrajectoryCritic(tasks, routeGraph, trajectoryLLM),
	}
}
