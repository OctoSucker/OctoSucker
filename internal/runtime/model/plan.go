package model

import (
	"encoding/json"
	"fmt"
	"maps"
	"strings"

	rt "github.com/OctoSucker/octosucker/internal/runtime/toolrouting"
)

type Plan struct {
	Steps []*PlanStep `json:"steps"`
}

func NewPlan() *Plan {
	return &Plan{Steps: make([]*PlanStep, 0)}
}

type StepStatus string

const (
	StepStatusPending StepStatus = "pending"
	StepStatusRunning StepStatus = "running"
	StepStatusDone    StepStatus = "done"
)

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
			if st.ToolResult.Kind != "" || st.ToolResult.Tool != "" {
				fmt.Fprintf(&b, "Tool result meta: kind=%s count=%d empty=%v tool=%s\n",
					st.ToolResult.Kind, st.ToolResult.Count, st.ToolResult.Empty, st.ToolResult.Tool)
			}
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

func (p *Plan) StepCount() int {
	if p == nil {
		return 0
	}
	return len(p.Steps)
}

func (p *Plan) HasSteps() bool {
	return p.StepCount() > 0
}

func (p *Plan) LastStep() *PlanStep {
	if p == nil || len(p.Steps) == 0 {
		return nil
	}
	return p.Steps[len(p.Steps)-1]
}

func (p *Plan) LastNode() rt.Node {
	last := p.LastStep()
	if last == nil {
		return rt.Node{}
	}
	return last.Node
}

func (p *Plan) FindPrevStep(stepID string) *PlanStep {
	if p == nil {
		return nil
	}
	for i := range p.Steps {
		if p.Steps[i].ID == stepID {
			if i == 0 {
				return nil
			}
			return p.Steps[i-1]
		}
	}
	return nil
}

type PlanStep struct {
	ID        string         `json:"id"`
	Goal      string         `json:"goal"`
	Status    StepStatus     `json:"status"`
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
	return p.UserReply()
}

func (p *Plan) UserReply() (string, error) {
	if p == nil || len(p.Steps) == 0 {
		return "", fmt.Errorf("plan: no steps for user reply")
	}
	_, txt, err := p.LastDoneStepPrimary()
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(txt) == "" {
		return "", fmt.Errorf("plan: no completed step output for user reply")
	}
	return txt, nil
}

func (p *Plan) DebugSummary() string {
	if p == nil || len(p.Steps) == 0 {
		return "(no plan steps)"
	}
	var b strings.Builder
	for i, st := range p.Steps {
		if st == nil {
			continue
		}
		if i > 0 {
			b.WriteString(" | ")
		}
		fmt.Fprintf(&b, "#%d status=%s tool=%s", i+1, st.Status, st.Node.Tool)
		if len(st.Arguments) > 0 {
			if argBytes, err := json.Marshal(st.Arguments); err == nil {
				fmt.Fprintf(&b, " args=%s", string(argBytes))
			}
		}
		if st.ToolResult.Err != nil {
			fmt.Fprintf(&b, " err=%s", truncateRunes(st.ToolResult.Err.Error(), 180))
			continue
		}
		if st.ToolResult.Kind != "" || st.ToolResult.Tool != "" {
			fmt.Fprintf(&b, " result={kind:%s count:%d empty:%v}", st.ToolResult.Kind, st.ToolResult.Count, st.ToolResult.Empty)
		}
	}
	return b.String()
}

// StepSummariesFromPlan maps done step id → PrimaryText for template substitution.
func StepSummariesFromPlan(p *Plan) (map[string]string, error) {
	out := make(map[string]string)
	if p == nil {
		return out, nil
	}
	for _, st := range p.Steps {
		if st.Status != StepStatusDone {
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
	return p.LastDoneStepPrimary()
}

func (p *Plan) LastDoneStepPrimary() (stepID string, text string, err error) {
	st := p.LastDoneStep()
	if st == nil {
		return "", "", nil
	}
	return st.ID, st.PrimaryText(), nil
}

func (p *Plan) LastDoneStep() *PlanStep {
	if p == nil {
		return nil
	}
	for i := len(p.Steps) - 1; i >= 0; i-- {
		if p.Steps[i].Status == StepStatusDone {
			return p.Steps[i]
		}
	}
	return nil
}

func (p *Plan) TruncateFromStep(stepID string) error {
	if stepID == "" {
		p.Steps = make([]*PlanStep, 0)
		return nil
	}
	if p == nil || len(p.Steps) == 0 {
		return fmt.Errorf("plan: cannot truncate from step %q", stepID)
	}
	cut := -1
	for i := range p.Steps {
		if p.Steps[i].ID == stepID {
			cut = i
			break
		}
	}
	if cut < 0 {
		return fmt.Errorf("plan: step %q not found", stepID)
	}
	p.Steps = p.Steps[:cut]
	return nil
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
			if p.Steps[j].Status != StepStatusDone {
				continue outer
			}
		}
		if p.Steps[i].Status == StepStatusPending {
			return []*PlanStep{p.Steps[i]}
		}
	}
	return nil
}

func (p *Plan) MarkRunning(stepID string) {
	for i := range p.Steps {
		if p.Steps[i].ID == stepID {
			p.Steps[i].Status = StepStatusRunning
			return
		}
	}
}

func (p *Plan) MarkDone(stepID string) {
	for i := range p.Steps {
		if p.Steps[i].ID == stepID {
			p.Steps[i].Status = StepStatusDone
			return
		}
	}
}

func (p *Plan) MarkPending(stepID string) {
	for i := range p.Steps {
		if p.Steps[i].ID == stepID {
			p.Steps[i].Status = StepStatusPending
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
		if p.Steps[i].Status != StepStatusDone {
			return false
		}
	}
	return true
}
