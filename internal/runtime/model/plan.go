package model

import (
	"fmt"
	"strings"
)

type PlanStepStatus string

const (
	PlanStepPending   PlanStepStatus = "pending"
	PlanStepRunning   PlanStepStatus = "running"
	PlanStepCompleted PlanStepStatus = "completed"
	PlanStepFailed    PlanStepStatus = "failed"
)

type PlanStep struct {
	ID              string         `json:"id"`
	Goal            string         `json:"goal"`
	SuccessCriteria string         `json:"success_criteria"`
	Status          PlanStepStatus `json:"status"`
	Evidence        string         `json:"evidence,omitempty"`
}

type Plan struct {
	Objective string     `json:"objective"`
	Revision  int        `json:"revision"`
	Steps     []PlanStep `json:"steps"`
}

func (t *Turn) RevisePlan(steps []PlanStep) error {
	if t == nil {
		return fmt.Errorf("turn is nil")
	}
	if len(steps) == 0 || len(steps) > 8 {
		return fmt.Errorf("plan requires 1 to 8 steps")
	}
	seen := make(map[string]struct{}, len(steps))
	for i := range steps {
		steps[i].ID = strings.TrimSpace(steps[i].ID)
		steps[i].Goal = strings.TrimSpace(steps[i].Goal)
		steps[i].SuccessCriteria = strings.TrimSpace(steps[i].SuccessCriteria)
		if steps[i].ID == "" || steps[i].Goal == "" || steps[i].SuccessCriteria == "" {
			return fmt.Errorf("plan step %d requires id, goal, and success_criteria", i+1)
		}
		if _, exists := seen[steps[i].ID]; exists {
			return fmt.Errorf("plan step id %q is duplicated", steps[i].ID)
		}
		seen[steps[i].ID] = struct{}{}
		steps[i].Status = PlanStepPending
		steps[i].Evidence = ""
	}

	completed := make([]PlanStep, 0)
	revision := 1
	if t.Plan != nil {
		revision = t.Plan.Revision + 1
		for _, old := range t.Plan.Steps {
			if old.Status == PlanStepCompleted {
				completed = append(completed, old)
			}
		}
	}
	completedIDs := make(map[string]struct{}, len(completed))
	for _, step := range completed {
		completedIDs[step.ID] = struct{}{}
	}
	merged := append([]PlanStep(nil), completed...)
	for _, step := range steps {
		if _, exists := completedIDs[step.ID]; !exists {
			merged = append(merged, step)
		}
	}
	t.Plan = &Plan{Objective: t.Goal, Revision: revision, Steps: merged}
	t.notifyPlan()
	return nil
}

func (t *Turn) PlanStepCompleted(id string) bool {
	if t == nil || t.Plan == nil {
		return false
	}
	for _, step := range t.Plan.Steps {
		if step.ID == strings.TrimSpace(id) {
			return step.Status == PlanStepCompleted
		}
	}
	return false
}

// PlanCanComplete reports whether the current action satisfying its criteria
// would leave no other incomplete plan steps.
func (t *Turn) PlanCanComplete(currentStepID string, currentSatisfied bool) bool {
	if t == nil || t.Plan == nil {
		return true
	}
	for _, step := range t.Plan.Steps {
		if step.ID == currentStepID {
			if !currentSatisfied {
				return false
			}
			continue
		}
		if step.Status != PlanStepCompleted {
			return false
		}
	}
	return true
}

func (t *Turn) updatePlanStep(action Action, assessment Assessment, executionErr error) {
	if t == nil || t.Plan == nil || strings.TrimSpace(action.PlanStepID) == "" {
		return
	}
	for i := range t.Plan.Steps {
		step := &t.Plan.Steps[i]
		if step.ID != action.PlanStepID {
			continue
		}
		switch {
		case executionErr != nil || assessment.RoutingOutcome == RoutingWrongRoute:
			step.Status = PlanStepFailed
		case assessment.CriteriaSatisfied:
			step.Status = PlanStepCompleted
			step.Evidence = strings.TrimSpace(assessment.Evidence)
		default:
			step.Status = PlanStepPending
		}
		t.notifyPlan()
		return
	}
}

func (t *Turn) markPlanStepRunning(action Action) {
	if t == nil || t.Plan == nil {
		return
	}
	for i := range t.Plan.Steps {
		if t.Plan.Steps[i].ID == action.PlanStepID {
			t.Plan.Steps[i].Status = PlanStepRunning
			t.notifyPlan()
			return
		}
	}
}

func (t *Turn) notifyPlan() {
	if t != nil && t.OnPlanChanged != nil && t.Plan != nil {
		copyPlan := *t.Plan
		copyPlan.Steps = append([]PlanStep(nil), t.Plan.Steps...)
		t.OnPlanChanged(&copyPlan)
	}
}
