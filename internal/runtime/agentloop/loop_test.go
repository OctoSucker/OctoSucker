package agentloop

import (
	"context"
	"errors"
	"testing"

	"github.com/OctoSucker/octosucker/internal/runtime/model"
	"github.com/OctoSucker/octosucker/internal/toolcontract"
)

type scriptedPlanner struct {
	decisions []model.Decision
	calls     int
}

func (p *scriptedPlanner) Decide(context.Context, *model.Turn) (model.Decision, error) {
	if p.calls >= len(p.decisions) {
		return model.Decision{}, errors.New("unexpected planner call")
	}
	d := p.decisions[p.calls]
	p.calls++
	return d, nil
}

type scriptedExecutor struct {
	observations []model.Observation
	calls        int
}

func (e *scriptedExecutor) Execute(context.Context, model.Action) model.Observation {
	o := e.observations[e.calls]
	e.calls++
	return o
}

type scriptedEvaluator struct {
	assessments []model.Assessment
	calls       int
}

func (e *scriptedEvaluator) Evaluate(context.Context, *model.Turn) (model.Assessment, error) {
	a := e.assessments[e.calls]
	e.calls++
	return a, nil
}

type fixedResponder struct{ answer string }

func (r fixedResponder) Respond(context.Context, *model.Turn, string, model.ResponseDisposition) (string, error) {
	return r.answer, nil
}

func act(tool string) model.Decision {
	return model.Decision{Kind: model.DecisionAct, Action: model.Action{Tool: tool, Goal: "run " + tool}}
}

func assessment(progress model.Progress, outcome model.RoutingOutcome) model.Assessment {
	return model.Assessment{Progress: progress, RoutingOutcome: outcome, RoutingReason: model.RoutingReasonNecessaryPrerequisite, Summary: string(progress)}
}

func TestRunContinuesThenCompletes(t *testing.T) {
	planner := &scriptedPlanner{decisions: []model.Decision{act("first"), act("second")}}
	executor := &scriptedExecutor{observations: []model.Observation{
		{Result: toolcontract.Result{Output: "one"}.WithInferredMeta("first")},
		{Result: toolcontract.Result{Output: "two"}.WithInferredMeta("second")},
	}}
	evaluator := &scriptedEvaluator{assessments: []model.Assessment{
		assessment(model.ProgressContinue, model.RoutingHelpful),
		assessment(model.ProgressComplete, model.RoutingHelpful),
	}}
	loop, err := New(planner, executor, evaluator, fixedResponder{answer: "done"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	turn := model.NewTurn("turn", "conversation", "goal", nil)

	if err := loop.Run(context.Background(), turn); err != nil {
		t.Fatal(err)
	}
	if turn.Status != model.TurnCompleted || turn.Answer != "done" {
		t.Fatalf("turn = status %q answer %q", turn.Status, turn.Answer)
	}
	if len(turn.Steps) != 3 { // two tools plus the response step
		t.Fatalf("steps = %d; want 3", len(turn.Steps))
	}
}

func TestTypedApprovalFailureBlocksWithoutLearningWrongRoute(t *testing.T) {
	planner := &scriptedPlanner{decisions: []model.Decision{act("dangerous")}}
	executor := &scriptedExecutor{observations: []model.Observation{{Result: toolcontract.Result{
		Err: toolcontract.ApprovalRequiredError("dangerous"),
	}.WithInferredMeta("dangerous")}}}
	evaluator := &scriptedEvaluator{}
	loop, _ := New(planner, executor, evaluator, fixedResponder{answer: "approval required"}, nil)
	turn := model.NewTurn("turn", "conversation", "goal", nil)

	if err := loop.Run(context.Background(), turn); err != nil {
		t.Fatal(err)
	}
	if turn.Status != model.TurnBlocked {
		t.Fatalf("status = %q; want blocked", turn.Status)
	}
	if got := turn.Steps[0].Assessment.RoutingOutcome; got != model.RoutingNoSignal {
		t.Fatalf("routing outcome = %q; want no_signal", got)
	}
	if evaluator.calls != 0 {
		t.Fatalf("evaluator calls = %d; want 0", evaluator.calls)
	}
}

func TestThreeWrongRoutesStopTheTurn(t *testing.T) {
	planner := &scriptedPlanner{decisions: []model.Decision{act("one"), act("two"), act("three")}}
	executor := &scriptedExecutor{observations: []model.Observation{
		{Result: toolcontract.Result{Output: "one"}.WithInferredMeta("one")},
		{Result: toolcontract.Result{Output: "two"}.WithInferredMeta("two")},
		{Result: toolcontract.Result{Output: "three"}.WithInferredMeta("three")},
	}}
	evaluator := &scriptedEvaluator{assessments: []model.Assessment{
		assessment(model.ProgressContinue, model.RoutingWrongRoute),
		assessment(model.ProgressContinue, model.RoutingWrongRoute),
		assessment(model.ProgressContinue, model.RoutingWrongRoute),
	}}
	loop, _ := New(planner, executor, evaluator, fixedResponder{answer: "blocked"}, nil)
	turn := model.NewTurn("turn", "conversation", "goal", nil)

	if err := loop.Run(context.Background(), turn); err != nil {
		t.Fatal(err)
	}
	if turn.Status != model.TurnBlocked {
		t.Fatalf("status = %q; want blocked", turn.Status)
	}
}

func TestExplicitPlanPreventsPrematureCompletion(t *testing.T) {
	first := act("first")
	first.Action.PlanStepID = "one"
	second := act("second")
	second.Action.PlanStepID = "two"
	planner := &scriptedPlanner{decisions: []model.Decision{first, second}}
	executor := &scriptedExecutor{observations: []model.Observation{
		{Result: toolcontract.Result{Output: "one"}.WithInferredMeta("first")},
		{Result: toolcontract.Result{Output: "two"}.WithInferredMeta("second")},
	}}
	evaluator := &scriptedEvaluator{assessments: []model.Assessment{
		{Progress: model.ProgressComplete, RoutingOutcome: model.RoutingHelpful, RoutingReason: model.RoutingReasonGoalSatisfied, Summary: "one done", CriteriaSatisfied: true, Evidence: "one"},
		{Progress: model.ProgressComplete, RoutingOutcome: model.RoutingHelpful, RoutingReason: model.RoutingReasonGoalSatisfied, Summary: "two done", CriteriaSatisfied: true, Evidence: "two"},
	}}
	loop, _ := New(planner, executor, evaluator, fixedResponder{answer: "done"}, nil)
	turn := model.NewTurn("turn", "conversation", "goal", nil)
	if err := turn.RevisePlan([]model.PlanStep{
		{ID: "one", Goal: "one", SuccessCriteria: "one done"},
		{ID: "two", Goal: "two", SuccessCriteria: "two done"},
	}); err != nil {
		t.Fatal(err)
	}

	if err := loop.Run(context.Background(), turn); err != nil {
		t.Fatal(err)
	}
	if planner.calls != 2 {
		t.Fatalf("planner calls = %d; want 2", planner.calls)
	}
	if turn.Plan.Steps[0].Status != model.PlanStepCompleted || turn.Plan.Steps[1].Status != model.PlanStepCompleted {
		t.Fatalf("plan = %#v", turn.Plan)
	}
}
