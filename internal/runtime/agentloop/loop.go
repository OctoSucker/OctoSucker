// Package agentloop owns the serial turn lifecycle. Domain workflows belong in
// tools and skills; this controller knows only actions, observations, progress,
// and termination.
package agentloop

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/OctoSucker/octosucker/internal/runtime/model"
)

const (
	maxActions             = 12
	maxConsecutiveFailures = 3
)

type Loop struct {
	planner   Planner
	executor  Executor
	evaluator Evaluator
	responder Responder
	learner   Learner
}

func New(planner Planner, executor Executor, evaluator Evaluator, responder Responder, learner Learner) (*Loop, error) {
	if planner == nil || executor == nil || evaluator == nil || responder == nil {
		return nil, fmt.Errorf("agent loop: planner, executor, evaluator, and responder are required")
	}
	return &Loop{planner: planner, executor: executor, evaluator: evaluator, responder: responder, learner: learner}, nil
}

func (l *Loop) Run(ctx context.Context, turn *model.Turn) error {
	if turn == nil {
		return fmt.Errorf("agent loop: turn is nil")
	}
	for actionCount := 0; actionCount < maxActions; {
		decision, err := l.planner.Decide(ctx, turn)
		if err != nil {
			return fmt.Errorf("agent loop: plan: %w", err)
		}
		turn.AppendTrace("planner decision=%s tool=%s goal=%s reason=%s", decision.Kind, decision.Action.Tool, decision.Action.Goal, decision.Reason)

		if decision.Kind == model.DecisionRespond {
			return l.finish(ctx, turn, decision.Reason, decision.Disposition, decision.Step)
		}
		if decision.Kind != model.DecisionAct {
			return fmt.Errorf("agent loop: unsupported decision %q", decision.Kind)
		}

		actionCount++
		step := turn.BeginStep(decision.Action)
		step.Title = decision.Step.Title
		step.Summary = decision.Step.Summary
		turn.NotifyStep(step)
		observation := l.executor.Execute(ctx, decision.Action)
		step.Observation = observation
		if observation.Result.Err == nil {
			if err := turn.ApplyContextArtifacts(observation.Result.Artifacts); err != nil {
				observation.Result.Err = fmt.Errorf("agent loop: apply tool context: %w", err)
				observation.Result = observation.Result.WithInferredMeta(decision.Action.Tool)
			}
		}
		turn.AppendTrace("tool observed tool=%s kind=%s count=%d empty=%v error=%v",
			decision.Action.Tool,
			observation.Result.Kind,
			observation.Result.Count,
			observation.Result.Empty,
			observation.Result.Err,
		)

		if observation.Result.Err != nil {
			step.Assessment = classifyToolFailure(observation.Result.Err)
			turn.CompleteStep(step, observation.Result.Err)
			turn.ConsecutiveFailures++
			l.learn(turn, step)
			if step.Assessment.Progress == model.ProgressBlocked || turn.ConsecutiveFailures >= maxConsecutiveFailures {
				reason := step.Assessment.Summary
				if turn.ConsecutiveFailures >= maxConsecutiveFailures && step.Assessment.Progress != model.ProgressBlocked {
					reason = fmt.Sprintf("Stopped after %d consecutive failed strategies. Last failure: %s", maxConsecutiveFailures, observation.Result.Err)
				}
				return l.finish(ctx, turn, reason, model.ResponseBlocked)
			}
			continue
		}

		assessment, err := l.evaluator.Evaluate(ctx, turn)
		if err != nil {
			log.Printf("agent loop: evaluator unavailable turn=%s err=%v", turn.ID, err)
			assessment = model.Assessment{
				Progress:       model.ProgressContinue,
				RoutingOutcome: model.RoutingNoSignal,
				RoutingReason:  model.RoutingReasonEvaluationUnavailable,
				Summary:        "Progress evaluation was unavailable; inspect the successful observation and decide whether to respond or take another action.",
				NextStepHint:   "Use the existing observation directly; do not repeat the same action.",
			}
		}
		step.Assessment = assessment
		turn.CompleteStep(step, nil)
		turn.AppendTrace("evaluation progress=%s routing_outcome=%s routing_reason=%s summary=%s", assessment.Progress, assessment.RoutingOutcome, assessment.RoutingReason, assessment.Summary)
		l.learn(turn, step)

		if assessment.RoutingOutcome == model.RoutingWrongRoute {
			turn.ConsecutiveFailures++
		} else {
			turn.ConsecutiveFailures = 0
		}
		if turn.ConsecutiveFailures >= maxConsecutiveFailures {
			return l.finish(ctx, turn, "Stopped after repeated actions that did not advance the user goal.", model.ResponseBlocked)
		}

		switch assessment.Progress {
		case model.ProgressComplete:
			return l.finish(ctx, turn, assessment.Summary, model.ResponseAnswer)
		case model.ProgressBlocked:
			return l.finish(ctx, turn, assessment.Summary, model.ResponseBlocked)
		case model.ProgressContinue:
			continue
		default:
			return fmt.Errorf("agent loop: evaluator returned unknown progress %q", assessment.Progress)
		}
	}
	return l.finish(ctx, turn, fmt.Sprintf("Stopped after reaching the action limit (%d).", maxActions), model.ResponseBlocked)
}

func (l *Loop) finish(ctx context.Context, turn *model.Turn, reason string, disposition model.ResponseDisposition, descriptions ...model.DecisionStep) error {
	description := model.DecisionStep{}
	if len(descriptions) > 0 {
		description = descriptions[0]
	}
	if strings.TrimSpace(description.Title) == "" {
		switch disposition {
		case model.ResponseClarify:
			description.Title = "收集任务所需信息"
		case model.ResponseBlocked:
			description.Title = "检查任务可执行性"
		default:
			description.Title = "整理并生成任务结果"
		}
	}
	answer, err := l.responder.Respond(ctx, turn, reason, disposition)
	if err != nil {
		step := turn.BeginResponseStep(description, disposition)
		turn.FinishResponseStep(step, disposition, err)
		return fmt.Errorf("agent loop: respond: %w", err)
	}
	step := turn.BeginResponseStep(description, disposition)
	switch disposition {
	case model.ResponseBlocked:
		turn.Block(answer, reason)
	case model.ResponseClarify:
		turn.AwaitInput(answer, reason)
	default:
		turn.Complete(answer, reason)
	}
	turn.FinishResponseStep(step, disposition, nil)
	turn.AppendTrace("turn finished status=%s reason=%s", turn.Status, reason)
	return nil
}

func (l *Loop) learn(turn *model.Turn, step *model.Step) {
	if l.learner == nil || turn == nil || step == nil || step.Assessment.RoutingOutcome == model.RoutingNoSignal {
		return
	}
	previous := ""
	if len(turn.Steps) >= 2 {
		previous = turn.Steps[len(turn.Steps)-2].Action.Tool
	}
	if err := l.learner.RecordOutcome(turn.Goal, previous, step.Action.Tool, step.Assessment.RoutingOutcome); err != nil {
		log.Printf("agent loop: routing learning ignored turn=%s err=%v", turn.ID, err)
	}
}

func classifyToolFailure(err error) model.Assessment {
	summary := "The tool action failed: " + strings.TrimSpace(err.Error())
	progress := model.ProgressContinue
	for _, marker := range []string{
		"unknown tool",
		"forbidden by blacklist",
		"unknown backend",
		"cannot connect to the docker daemon",
		"no such file or directory",
		"executable file not found",
		"command not found",
		"not on path",
		"executable is not available",
		"permission denied",
		"rejected by user",
		"operation not permitted",
		"missing required environment",
		"unauthorized",
		"forbidden",
		"exceeded your current quota",
		"too many requests",
	} {
		if strings.Contains(strings.ToLower(summary), marker) {
			progress = model.ProgressBlocked
			break
		}
	}
	return model.Assessment{
		Progress:       progress,
		RoutingOutcome: model.RoutingWrongRoute,
		RoutingReason:  model.RoutingReasonTechnicalError,
		Summary:        summary,
		NextStepHint:   "Choose a materially different action only when the failure is recoverable.",
	}
}
