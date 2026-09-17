package model

import "testing"

func TestPlanTracksRunningAndEvidenceBackedCompletion(t *testing.T) {
	turn := NewTurn("turn", "conversation", "goal", nil)
	if err := turn.RevisePlan([]PlanStep{
		{ID: "collect", Goal: "collect data", SuccessCriteria: "data returned"},
		{ID: "write", Goal: "write report", SuccessCriteria: "report path returned"},
	}); err != nil {
		t.Fatal(err)
	}
	action := Action{PlanStepID: "collect", Goal: "collect data", SuccessCriteria: "data returned"}
	step := turn.BeginStep(action)
	if turn.Plan.Steps[0].Status != PlanStepRunning {
		t.Fatalf("status = %q; want running", turn.Plan.Steps[0].Status)
	}
	step.Assessment = Assessment{RoutingOutcome: RoutingHelpful, CriteriaSatisfied: true, Evidence: "tool returned 3 records"}
	turn.CompleteStep(step, nil)
	if turn.Plan.Steps[0].Status != PlanStepCompleted || turn.Plan.Steps[0].Evidence == "" {
		t.Fatalf("completed plan step = %#v", turn.Plan.Steps[0])
	}
}

func TestPlanRevisionPreservesCompletedSteps(t *testing.T) {
	turn := NewTurn("turn", "conversation", "goal", nil)
	if err := turn.RevisePlan([]PlanStep{{ID: "one", Goal: "one", SuccessCriteria: "one done"}}); err != nil {
		t.Fatal(err)
	}
	turn.Plan.Steps[0].Status = PlanStepCompleted
	turn.Plan.Steps[0].Evidence = "evidence one"
	if err := turn.RevisePlan([]PlanStep{{ID: "two", Goal: "two", SuccessCriteria: "two done"}}); err != nil {
		t.Fatal(err)
	}
	if turn.Plan.Revision != 2 || len(turn.Plan.Steps) != 2 {
		t.Fatalf("revised plan = %#v", turn.Plan)
	}
	if turn.Plan.Steps[0].Status != PlanStepCompleted || turn.Plan.Steps[0].Evidence != "evidence one" {
		t.Fatalf("completed step was not preserved: %#v", turn.Plan.Steps[0])
	}
}

func TestPlanRejectsDuplicateStepIDs(t *testing.T) {
	turn := NewTurn("turn", "conversation", "goal", nil)
	err := turn.RevisePlan([]PlanStep{
		{ID: "same", Goal: "one", SuccessCriteria: "one done"},
		{ID: "same", Goal: "two", SuccessCriteria: "two done"},
	})
	if err == nil {
		t.Fatal("expected duplicate id error")
	}
}

func TestPlanCannotCompleteWhileAnotherStepIsPending(t *testing.T) {
	turn := NewTurn("turn", "conversation", "goal", nil)
	if err := turn.RevisePlan([]PlanStep{
		{ID: "one", Goal: "one", SuccessCriteria: "one done"},
		{ID: "two", Goal: "two", SuccessCriteria: "two done"},
	}); err != nil {
		t.Fatal(err)
	}
	if turn.PlanCanComplete("one", true) {
		t.Fatal("plan completed with another pending step")
	}
	turn.Plan.Steps[1].Status = PlanStepCompleted
	if !turn.PlanCanComplete("one", true) {
		t.Fatal("plan did not complete when all criteria were satisfied")
	}
}
