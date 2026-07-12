package agentloop

import (
	"context"

	"github.com/OctoSucker/octosucker/internal/runtime/model"
)

type Planner interface {
	Decide(ctx context.Context, turn *model.Turn) (model.Decision, error)
}

type Executor interface {
	Execute(ctx context.Context, action model.Action) model.Observation
}

type Evaluator interface {
	Evaluate(ctx context.Context, turn *model.Turn) (model.Assessment, error)
}

type Responder interface {
	Respond(ctx context.Context, turn *model.Turn, terminalReason string, disposition model.ResponseDisposition) (string, error)
}

type Learner interface {
	RecordOutcome(goal, previousTool, currentTool string, outcome model.RoutingOutcome) error
}
