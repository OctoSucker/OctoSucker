// Task state persisted per executor turn.
package types

import (
	"fmt"
	"strings"
)

type Task struct {
	ID                string `json:"id"` // UUID；SQLite PRIMARY KEY
	UserInput         string `json:"user_input"`
	Plan              *Plan  `json:"plan,omitempty"`
	Reply             string `json:"reply"`
	TrajectorySummary string `json:"trajectory_summary,omitempty"`
	ReplanCount       int    `json:"replan_count,omitempty"`
}

// TruncatePlanFromStep adjusts plan for replanning. Two modes:
//   - failedStepID non-empty (StepCritic): remove that step and all following; keep prefix. Empty prefix clears plan and resets RouteSnap to entry.
//   - failedStepID empty (TrajectoryCritic): discard the entire plan and reset RouteSnap to entry (full replan). StepCritic must pass a concrete step id.
func (t *Task) TruncatePlanFromStep(failedStepID string) error {
	if failedStepID == "" {
		t.Plan = &Plan{
			Steps: make([]*PlanStep, 0),
		}
		return nil
	} else {
		if t.Plan == nil || len(t.Plan.Steps) == 0 {
			return fmt.Errorf("task: cannot truncate plan (failed step %q)", failedStepID)
		}
		cut := -1
		for i := range t.Plan.Steps {
			if t.Plan.Steps[i].ID == failedStepID {
				cut = i
				break
			}
		}
		if cut < 0 {
			return fmt.Errorf("task: failed step %q not found in plan", failedStepID)
		}
		t.Plan.Steps = t.Plan.Steps[:cut]
		if len(t.Plan.Steps) == 0 {
			t.Plan = &Plan{
				Steps: make([]*PlanStep, 0),
			}
			return nil
		}
		return nil
	}
}

// UserFacingTurnMessages returns chat bubbles: trace-derived Reply first, then TrajectorySummary when present.
// Raw field values are appended when the trimmed form is non-empty (same as historical Telegram behavior).
func (t *Task) UserFacingTurnMessages() ([]string, error) {
	reply := strings.ReplaceAll(t.Reply, `\n`, "\n")
	summary := strings.ReplaceAll(t.TrajectorySummary, `\n`, "\n")
	r := strings.TrimSpace(reply)
	s := strings.TrimSpace(summary)
	if r == "" && s == "" {
		return nil, fmt.Errorf("task has empty reply")
	}
	var out []string
	if r != "" {
		out = append(out, reply)
	}
	if s != "" {
		out = append(out, summary)
	}
	return out, nil
}
