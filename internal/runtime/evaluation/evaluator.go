// Package evaluation judges whether the latest successful observation moved
// the whole turn toward its goal. It never invokes tools or writes answers.
package evaluation

import (
	"context"
	"fmt"
	"strings"

	"github.com/OctoSucker/octosucker/internal/runtime/contextmanager"
	"github.com/OctoSucker/octosucker/internal/runtime/model"
)

const evaluationAttempts = 2

type JSONCompleter interface {
	CompleteJSON(ctx context.Context, system, user string, out any) error
}

type Evaluator struct {
	llm        JSONCompleter
	contexts   *contextmanager.Manager
	projectCtx string
}

func New(llm JSONCompleter, contexts *contextmanager.Manager, projectCtx string) (*Evaluator, error) {
	if llm == nil {
		return nil, fmt.Errorf("evaluator: llm is required")
	}
	if contexts == nil {
		return nil, fmt.Errorf("evaluator: context manager is required")
	}
	return &Evaluator{llm: llm, contexts: contexts, projectCtx: strings.TrimSpace(projectCtx)}, nil
}

type assessmentJSON struct {
	Progress       string `json:"progress"`
	RoutingOutcome string `json:"routing_outcome"`
	RoutingReason  string `json:"routing_reason"`
	Summary        string `json:"summary"`
	NextStepHint   string `json:"next_step_hint"`
}

func (e *Evaluator) Evaluate(ctx context.Context, turn *model.Turn) (model.Assessment, error) {
	if turn == nil || turn.LastStep() == nil {
		return model.Assessment{}, fmt.Errorf("evaluator: turn has no observation")
	}
	if turn.LastStep().Observation.Result.Err != nil {
		return model.Assessment{}, fmt.Errorf("evaluator: tool failures are classified by the loop")
	}

	feedback := ""
	var lastErr error
	for attempt := 0; attempt < evaluationAttempts; attempt++ {
		system, user, stats := e.prompts(turn, feedback)
		if attempt == 0 {
			turn.AppendTrace("%s", stats.Trace())
		}
		var raw assessmentJSON
		if err := e.llm.CompleteJSON(ctx, system, user, &raw); err != nil {
			lastErr = fmt.Errorf("evaluator: assessment json: %w", err)
			feedback = lastErr.Error()
			continue
		}
		assessment, err := normalizeAssessment(raw)
		if err == nil {
			return assessment, nil
		}
		lastErr = err
		feedback = err.Error()
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("evaluator: no valid assessment")
	}
	return model.Assessment{}, lastErr
}

func normalizeAssessment(raw assessmentJSON) (model.Assessment, error) {
	progress := model.Progress(strings.ToLower(strings.TrimSpace(raw.Progress)))
	switch progress {
	case model.ProgressContinue, model.ProgressComplete, model.ProgressBlocked:
	default:
		return model.Assessment{}, fmt.Errorf("evaluator: unknown progress %q", raw.Progress)
	}
	routingOutcome := model.RoutingOutcome(strings.ToLower(strings.TrimSpace(raw.RoutingOutcome)))
	switch routingOutcome {
	case model.RoutingHelpful, model.RoutingWrongRoute, model.RoutingNoSignal:
	default:
		return model.Assessment{}, fmt.Errorf("evaluator: unknown routing outcome %q", raw.RoutingOutcome)
	}
	routingReason := model.RoutingReason(strings.ToLower(strings.TrimSpace(raw.RoutingReason)))
	switch routingReason {
	case model.RoutingReasonGoalSatisfied,
		model.RoutingReasonNecessaryPrerequisite,
		model.RoutingReasonIrrelevantResult,
		model.RoutingReasonWrongStrategy,
		model.RoutingReasonValidEmpty,
		model.RoutingReasonAmbiguousResult:
	default:
		return model.Assessment{}, fmt.Errorf("evaluator: unknown routing reason %q", raw.RoutingReason)
	}
	summary := strings.TrimSpace(raw.Summary)
	if summary == "" {
		return model.Assessment{}, fmt.Errorf("evaluator: summary is required")
	}
	return model.Assessment{
		Progress:       progress,
		RoutingOutcome: routingOutcome,
		RoutingReason:  routingReason,
		Summary:        summary,
		NextStepHint:   strings.TrimSpace(raw.NextStepHint),
	}, nil
}
