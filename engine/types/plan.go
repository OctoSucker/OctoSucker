package types

import (
	"encoding/json"
	"fmt"
	"maps"
	"strings"

	rt "github.com/OctoSucker/octosucker/repo/routegraph"
)

type Plan struct {
	Steps []*PlanStep `json:"steps"`
}

// FormatForPlannerPrompt renders executed/pending steps as readable lines for the LLM planner user message.
func (p *Plan) FormatForPlannerPrompt() (string, error) {
	if p == nil || len(p.Steps) == 0 {
		return "(none — no prior steps on this task)", nil
	}
	var b strings.Builder
	for i, st := range p.Steps {
		fmt.Fprintf(&b, "--- Step %d of %d ---\n", i+1, len(p.Steps))
		fmt.Fprintf(&b, "Step ID: %s\n", st.ID)
		fmt.Fprintf(&b, "Status: %s\n", st.Status)
		fmt.Fprintf(&b, "Goal: %s\n", st.Goal)
		fmt.Fprintf(&b, "Tool: %s\n", st.Node.Tool)
		argBytes, err := json.Marshal(st.Arguments)
		if err != nil {
			return "", fmt.Errorf("plan: marshal step arguments for prompt: %w", err)
		}
		fmt.Fprintf(&b, "Arguments JSON: %s\n", string(argBytes))
		if st.ToolResult.Err != nil {
			fmt.Fprintf(&b, "Tool error: %v\n", st.ToolResult.Err)
		} else {
			out := st.PrimaryText()
			if out != "" {
				fmt.Fprintf(&b, "Tool output (compact):\n%s\n", out)
			}
		}
	}
	return b.String(), nil
}

func (p *Plan) FindStep(stepID string) *PlanStep {
	if p == nil {
		return nil
	}
	for i := range p.Steps {
		if p.Steps[i].ID == stepID {
			return p.Steps[i]
		}
	}
	return nil
}

func (p *Plan) FindPrevStep(stepID string) *PlanStep {
	if p == nil {
		return nil
	}
	prevStep := &PlanStep{}
	for i := range p.Steps {
		if p.Steps[i].ID == stepID {
			return prevStep
		} else {
			prevStep = p.Steps[i]
		}
	}
	return nil
}

type PlanStep struct {
	ID        string         `json:"id"`
	Goal      string         `json:"goal"`
	Status    string         `json:"status"`
	Node      rt.Node        `json:"node"`
	Arguments map[string]any `json:"arguments,omitempty"`
	// ToolResult is this step's tool observation once it reaches status done. Failures before done only bump ToolFailStreak.
	ToolResult ToolResult `json:"result,omitempty"`
}

// PrimaryText is the same as ToolResult.CompactForLLM for this step’s stored tool output.
func (s *PlanStep) PrimaryText() string {
	if s == nil {
		return ""
	}

	out := s.ToolResult.CompactForLLM()
	return out
}

// UserReplyFromPlan concatenates non-empty PrimaryText from each done step in plan order.
func UserReplyFromPlan(p *Plan) (string, error) {
	return p.Steps[len(p.Steps)-1].PrimaryText(), nil
}

// StepSummariesFromPlan maps done step id → PrimaryText for template substitution.
func StepSummariesFromPlan(p *Plan) (map[string]string, error) {
	out := make(map[string]string)
	if p == nil {
		return out, nil
	}
	for _, st := range p.Steps {
		if st.Status != "done" {
			continue
		}
		txt := st.PrimaryText()
		if txt != "" {
			out[st.ID] = txt
		}
	}
	return out, nil
}

// LastDoneStepPrimary returns the id and PrimaryText of the last done step in slice order.
func LastDoneStepPrimary(p *Plan) (stepID string, text string, err error) {
	if p == nil {
		return "", "", nil
	}
	for i := len(p.Steps) - 1; i >= 0; i-- {
		if p.Steps[i].Status == "done" {
			txt := p.Steps[i].PrimaryText()
			return p.Steps[i].ID, txt, nil
		}
	}
	return "", "", nil
}

// Runnable returns the next step to run: the first pending step in slice order such that every
// earlier step is done. No concurrency — at most one pending step is runnable.
func (p *Plan) Runnable() []*PlanStep {
	if p == nil {
		return nil
	}
outer:
	for i := range p.Steps {
		for j := 0; j < i; j++ {
			if p.Steps[j].Status != "done" {
				continue outer
			}
		}
		if p.Steps[i].Status == "pending" {
			return []*PlanStep{p.Steps[i]}
		}
	}
	return nil
}

func (p *Plan) MarkRunning(stepID string) {
	for i := range p.Steps {
		if p.Steps[i].ID == stepID {
			p.Steps[i].Status = "running"
			return
		}
	}
}

func (p *Plan) MarkDone(stepID string) {
	for i := range p.Steps {
		if p.Steps[i].ID == stepID {
			p.Steps[i].Status = "done"
			return
		}
	}
}

func (p *Plan) MarkPending(stepID string) {
	for i := range p.Steps {
		if p.Steps[i].ID == stepID {
			p.Steps[i].Status = "pending"
			return
		}
	}
}

func (s PlanStep) Clone() PlanStep {
	out := s
	out.Arguments = maps.Clone(s.Arguments)
	return out
}

func (p *Plan) AllDone() bool {
	if p == nil || len(p.Steps) == 0 {
		return false
	}
	for i := range p.Steps {
		if p.Steps[i].Status != "done" {
			return false
		}
	}
	return true
}
