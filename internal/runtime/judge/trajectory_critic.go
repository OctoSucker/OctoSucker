package judge

import (
	"context"
	"fmt"
	"log"
	"strings"

	rt "github.com/OctoSucker/octosucker/internal/runtime/toolrouting"
	types "github.com/OctoSucker/octosucker/internal/runtime/model"
	"github.com/OctoSucker/octosucker/internal/runtime/taskstore"
	"github.com/OctoSucker/octosucker/pkg/llmclient"
)

const maxReplansPerTurn = 5

// Trajectory outcomes returned by the judge LLM (JSON field "outcome").
const (
	outcomeComplete = "complete" // user request satisfied; end turn
	outcomeContinue = "continue" // trajectory healthy; plan one more step
	outcomeAbort    = "abort"    // cannot satisfy request; end turn with rationale
	outcomeReplan   = "replan"   // discard bad suffix and replan; optional truncate_from_step_id
)

type TrajectoryEvaluator struct {
	Tasks       *taskstore.TaskStore
	RouteGraph  TransitionRecorder
	Judge       *TrajectoryEvaluatorJudge
	Synthesizer *FinalSynthesizer
	Applier     *TrajectoryDecisionApplier
}

func NewTrajectoryEvaluator(tasks *taskstore.TaskStore, routeGraph TransitionRecorder, trajectoryLLM *llmclient.OpenAI, projectCtx string) *TrajectoryEvaluator {
	return &TrajectoryEvaluator{
		Tasks:       tasks,
		RouteGraph:  routeGraph,
		Judge:       &TrajectoryEvaluatorJudge{LLM: trajectoryLLM, ProjectCtx: projectCtx},
		Synthesizer: &FinalSynthesizer{LLM: trajectoryLLM, ProjectCtx: projectCtx},
		Applier:     &TrajectoryDecisionApplier{},
	}
}

func (c *TrajectoryEvaluator) HandleTrajectoryCheck(ctx context.Context, pl types.TrajectoryEvaluationRequest) (*types.Event, error) {
	task, ok := c.Tasks.Get(pl.TaskID)
	if !ok {
		return nil, fmt.Errorf("trajectory_critic: task %q not found", pl.TaskID)
	}
	verdict, err := c.Judge.Evaluate(ctx, task)
	if err != nil {
		return nil, fmt.Errorf("trajectory_critic: verdict: %w", err)
	}
	if err := c.recordSemanticRouteLearning(task, verdict); err != nil {
		return nil, fmt.Errorf("trajectory_critic: semantic route learning: %w", err)
	}
	task.AppendTrace("trajectory outcome=%s next_phase=%s learning=%s rationale=%s", verdict.Outcome, verdict.NextPhase, verdict.LastStepLearningOutcome, clipForLog(verdict.Rationale, 220))
	finalAnswer, err := c.finalAnswer(ctx, task, verdict)
	if err != nil {
		return nil, fmt.Errorf("trajectory_critic: final answer: %w", err)
	}
	next, err := c.Applier.Apply(pl.TaskID, task, verdict, finalAnswer)
	if err != nil {
		return nil, fmt.Errorf("trajectory_critic: apply verdict: %w", err)
	}
	if err := c.Tasks.Put(task); err != nil {
		return nil, err
	}
	logTrajectoryOutcome(pl.TaskID, verdict.Outcome, task, verdict, next)
	return next, nil
}

func (c *TrajectoryEvaluator) finalAnswer(ctx context.Context, task *types.Task, verdict trajectoryVerdict) (string, error) {
	if verdict.Outcome != outcomeComplete && verdict.Outcome != outcomeAbort {
		return "", nil
	}
	if c == nil || c.Synthesizer == nil {
		return fallbackFinalAnswer(task, verdict)
	}
	return c.Synthesizer.Synthesize(ctx, task, verdict)
}

func (c *TrajectoryEvaluator) recordSemanticRouteLearning(task *types.Task, verdict trajectoryVerdict) error {
	if c == nil || c.RouteGraph == nil || task == nil || task.Plan == nil {
		return nil
	}
	success, ok := verdict.LastStepLearningSuccess()
	if !ok {
		return nil
	}
	step := task.Plan.LastDoneStep()
	if step == nil {
		return nil
	}
	prev := task.Plan.FindPrevStep(step.ID)
	prevNode := rt.Node{}
	if prev != nil {
		prevNode = prev.Node
	}
	return c.RouteGraph.RecordTransition(task.UserInput, 0, 0, prevNode, step.Node, success)
}

func logTrajectoryOutcome(taskID, outcome string, task *types.Task, v trajectoryVerdict, next *types.Event) {
	nextTyp := "nil"
	if next != nil {
		nextTyp = next.Type
	}
	nSteps, lastTool, lastStatus := 0, "", ""
	if task.Plan != nil {
		nSteps = task.Plan.StepCount()
		if nSteps > 0 {
			s := task.Plan.LastStep()
			lastTool = s.Node.Tool
			lastStatus = string(s.Status)
		}
	}
	log.Printf(
		"trajectory_critic: task=%s phase=%s outcome=%s next_phase=%s learning=%s next_evt=%s plan_steps=%d last_tool=%s last_status=%s replan_count=%d truncate=%q rationale=%s",
		taskID, task.Phase, outcome, v.NextPhase, v.LastStepLearningOutcome, nextTyp, nSteps, lastTool, lastStatus, task.ReplanCount, strings.TrimSpace(v.TruncateFromStepID), clipForLog(v.Rationale, 400),
	)
}

func clipForLog(s string, maxRunes int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= maxRunes {
		return string(r)
	}
	return string(r[:maxRunes]) + "…"
}

func normalizeTrajectoryOutcome(s string) (string, error) {
	o := strings.ToLower(strings.TrimSpace(s))
	switch o {
	case outcomeComplete, outcomeContinue, outcomeAbort, outcomeReplan:
		return o, nil
	default:
		return "", fmt.Errorf("unknown outcome %q (want %q, %q, %q, or %q)",
			s, outcomeComplete, outcomeContinue, outcomeAbort, outcomeReplan)
	}
}

func normalizeNextPhase(s, outcome string) (string, error) {
	p := strings.ToLower(strings.TrimSpace(s))
	if p == "" {
		switch outcome {
		case outcomeComplete:
			return string(types.TaskPhaseDone), nil
		case outcomeAbort:
			return string(types.TaskPhaseAbort), nil
		case outcomeReplan, outcomeContinue:
			return string(types.TaskPhaseExecution), nil
		}
	}
	switch types.TaskPhase(p) {
	case types.TaskPhaseDiscovery, types.TaskPhaseExecution, types.TaskPhaseSynthesis, types.TaskPhaseDone, types.TaskPhaseAbort:
		if outcome == outcomeComplete && p != string(types.TaskPhaseDone) {
			return "", fmt.Errorf("next_phase %q invalid for outcome %q", s, outcome)
		}
		if outcome == outcomeAbort && p != string(types.TaskPhaseAbort) {
			return "", fmt.Errorf("next_phase %q invalid for outcome %q", s, outcome)
		}
		if (outcome == outcomeContinue || outcome == outcomeReplan) && (p == string(types.TaskPhaseDone) || p == string(types.TaskPhaseAbort)) {
			return "", fmt.Errorf("next_phase %q invalid for outcome %q", s, outcome)
		}
		return p, nil
	default:
		return "", fmt.Errorf("unknown next_phase %q (want %q, %q, %q, %q, or %q)",
			s, types.TaskPhaseDiscovery, types.TaskPhaseExecution, types.TaskPhaseSynthesis, types.TaskPhaseDone, types.TaskPhaseAbort)
	}
}

func normalizeLastStepLearningOutcome(s, outcome string) (string, error) {
	o := strings.ToLower(strings.TrimSpace(s))
	switch o {
	case lastStepLearningSuccess, lastStepLearningNeutral, lastStepLearningFailure:
		return o, nil
	case "":
		switch outcome {
		case outcomeComplete, outcomeContinue:
			return lastStepLearningSuccess, nil
		case outcomeReplan:
			return lastStepLearningFailure, nil
		default:
			return lastStepLearningNeutral, nil
		}
	default:
		return "", fmt.Errorf("unknown last_step_learning_outcome %q (want %q, %q, or %q)",
			s, lastStepLearningSuccess, lastStepLearningNeutral, lastStepLearningFailure)
	}
}
