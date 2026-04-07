package judge

import (
	"github.com/OctoSucker/octosucker/repo/routegraph"
	"github.com/OctoSucker/octosucker/repo/taskstore"
	"github.com/OctoSucker/octosucker/pkg/llmclient"
)

// Judge owns post-execution evaluation and finalization handlers.
type Judge struct {
	StepCritic       *StepCritic
	TrajectoryCritic *TrajectoryCritic
}

func NewJudge(
	tasks *taskstore.TaskStore,
	routeGraph *routegraph.Graph,
	trajectoryLLM *llmclient.OpenAI,
) *Judge {
	return &Judge{
		StepCritic:       NewStepCritic(tasks, routeGraph),
		TrajectoryCritic: NewTrajectoryCritic(tasks, routeGraph, trajectoryLLM),
	}
}
