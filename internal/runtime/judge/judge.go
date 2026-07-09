package judge

import (
	"github.com/OctoSucker/octosucker/internal/runtime/taskstore"
	"github.com/OctoSucker/octosucker/pkg/llmclient"
)

// Evaluator owns post-execution evaluation and finalization handlers.
type Evaluator struct {
	StepEvaluator       *StepEvaluator
	TrajectoryEvaluator *TrajectoryEvaluator
}

func NewEvaluator(
	tasks *taskstore.TaskStore,
	routeGraph TransitionRecorder,
	trajectoryLLM *llmclient.OpenAI,
	projectCtx string,
) *Evaluator {
	return &Evaluator{
		StepEvaluator:       NewStepEvaluator(tasks, routeGraph),
		TrajectoryEvaluator: NewTrajectoryEvaluator(tasks, routeGraph, trajectoryLLM, projectCtx),
	}
}
