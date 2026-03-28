package ports

import "maps"

type Plan struct {
	Steps []PlanStep `json:"steps"`
}

func (p *Plan) FindStep(stepID string) *PlanStep {
	if p == nil {
		return nil
	}
	for i := range p.Steps {
		if p.Steps[i].ID == stepID {
			return &p.Steps[i]
		}
	}
	return nil
}

type PlanStep struct {
	ID         string   `json:"id"`
	Goal       string   `json:"goal"`
	Status     string   `json:"status"`
	DependsOn  []string `json:"depends_on"`
	Capability string   `json:"capability"`
	// Tool is the MCP tool name when the capability exposes multiple tools; empty means the first tool (legacy / single-tool caps).
	Tool      string         `json:"tool,omitempty"`
	Arguments map[string]any `json:"arguments,omitempty"`
}

func (p *Plan) Runnable() []PlanStep {
	done := make(map[string]bool)
	for i := range p.Steps {
		if p.Steps[i].Status == "done" {
			done[p.Steps[i].ID] = true
		}
	}
	var out []PlanStep
	for i := range p.Steps {
		s := p.Steps[i]
		if s.Status != "pending" && s.Status != "running" {
			continue
		}
		if s.Status == "running" {
			continue
		}
		ok := true
		for _, d := range s.DependsOn {
			if !done[d] {
				ok = false
				break
			}
		}
		if ok {
			out = append(out, s)
		}
	}
	return out
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
	out.DependsOn = append([]string(nil), s.DependsOn...)
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
