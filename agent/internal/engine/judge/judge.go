package judge

import (
	"github.com/OctoSucker/agent/pkg/llmclient"
	"github.com/OctoSucker/agent/repo/recall"
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
	recallCorpus *recall.RecallCorpus,
) *Judge {
	return &Judge{
		StepCritic:       NewStepCritic(tasks, routeGraph),
		TrajectoryCritic: NewTrajectoryCritic(tasks, routeGraph, recallCorpus, trajectoryLLM),
	}
}
