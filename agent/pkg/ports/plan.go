package ports

import (
	"encoding/json"
	"maps"
	"strings"
)

type Plan struct {
	Steps []*PlanStep `json:"steps"`
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

type PlanStep struct {
	ID         string `json:"id"`
	Goal       string `json:"goal"`
	Status     string `json:"status"`
	Capability string `json:"capability"`
	// Tool is the MCP tool name when the capability exposes multiple tools; empty means the first tool (legacy / single-tool caps).
	Tool      string         `json:"tool,omitempty"`
	Arguments map[string]any `json:"arguments,omitempty"`
	// Obs is this step's tool observation once it reaches status done. Failures before done only bump ToolFailStreak.
	Obs            Observation `json:"obs,omitempty"`
	ToolFailStreak int         `json:"tool_fail_streak,omitempty"`
}

// UnmarshalJSON supports legacy last_obs and last_obs_summary / last_obs_structured keys.
func (s *PlanStep) UnmarshalJSON(data []byte) error {
	var aux struct {
		ID               string         `json:"id"`
		Goal             string         `json:"goal"`
		Status           string         `json:"status"`
		Capability       string         `json:"capability"`
		Tool             string         `json:"tool,omitempty"`
		Arguments        map[string]any `json:"arguments,omitempty"`
		Obs              Observation    `json:"obs,omitempty"`
		LegacyLastObs    Observation    `json:"last_obs,omitempty"`
		ToolFailStreak   int            `json:"tool_fail_streak,omitempty"`
		LegacySummary    string         `json:"last_obs_summary,omitempty"`
		LegacyStructured any            `json:"last_obs_structured,omitempty"`
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	*s = PlanStep{
		ID: aux.ID, Goal: aux.Goal, Status: aux.Status, Capability: aux.Capability, Tool: aux.Tool,
		Arguments: aux.Arguments, ToolFailStreak: aux.ToolFailStreak,
	}
	switch {
	case !aux.Obs.isZero():
		s.Obs = aux.Obs
	case !aux.LegacyLastObs.isZero():
		s.Obs = aux.LegacyLastObs
	case aux.LegacySummary != "" || aux.LegacyStructured != nil:
		s.Obs = Observation{Summary: aux.LegacySummary, Structured: aux.LegacyStructured}
	}
	return nil
}

// PrimaryText for a done step prefers pretty JSON of Obs.Structured; otherwise Obs.Summary. Non-done steps return Obs.Summary only.
func (s *PlanStep) PrimaryText() string {
	if s.Status != "done" {
		return s.Obs.Summary
	}
	if s.Obs.Structured != nil {
		if str, ok := s.Obs.Structured.(string); ok {
			return str
		}
		if b, err := json.MarshalIndent(s.Obs.Structured, "", "  "); err == nil {
			return string(b)
		}
		if b, err := json.Marshal(s.Obs.Structured); err == nil {
			return string(b)
		}
	}
	return s.Obs.Summary
}

// UserReplyFromPlan concatenates non-empty PrimaryText from each done step in plan order.
func UserReplyFromPlan(p *Plan) string {
	var b strings.Builder
	for _, st := range p.Steps {
		if st.Status != "done" {
			continue
		}
		if txt := st.PrimaryText(); txt != "" {
			b.WriteString(txt)
			b.WriteString("\n")
		}
	}
	return strings.TrimSpace(b.String())
}

// StepSummariesFromPlan maps done step id → PrimaryText for template substitution.
func StepSummariesFromPlan(p *Plan) map[string]string {
	out := make(map[string]string)
	if p == nil {
		return out
	}
	for _, st := range p.Steps {
		if st.Status != "done" {
			continue
		}
		if txt := st.PrimaryText(); txt != "" {
			out[st.ID] = txt
		}
	}
	return out
}

// LastDoneStepPrimary returns the id and PrimaryText of the last done step in slice order.
func LastDoneStepPrimary(p *Plan) (stepID string, text string) {
	if p == nil {
		return "", ""
	}
	for i := len(p.Steps) - 1; i >= 0; i-- {
		if p.Steps[i].Status == "done" {
			return p.Steps[i].ID, p.Steps[i].PrimaryText()
		}
	}
	return "", ""
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
