package skill

import (
	"encoding/json"
	"fmt"
	"maps"
	"strings"

	"github.com/OctoSucker/agent/pkg/ports"
)

// InvocationContext carries everything needed to resolve skill parameters and {{...}} templates.
type InvocationContext struct {
	UserInput string
	Trace     []ports.StepTrace
	Plan      *ports.Plan
}

// StepSummariesByStepID aggregates trace summaries per step (OK rows joined with newline; if none OK, last failing summary).
func StepSummariesByStepID(tr []ports.StepTrace) map[string]string {
	type acc struct {
		okParts []string
		lastAny string
	}
	by := map[string]*acc{}
	for _, t := range tr {
		if t.StepID == "" {
			continue
		}
		a := by[t.StepID]
		if a == nil {
			a = &acc{}
			by[t.StepID] = a
		}
		if t.Summary != "" {
			a.lastAny = t.Summary
			if t.OK {
				a.okParts = append(a.okParts, t.Summary)
			}
		}
	}
	out := make(map[string]string, len(by))
	for id, a := range by {
		if len(a.okParts) > 0 {
			out[id] = strings.Join(a.okParts, "\n")
		} else if a.lastAny != "" {
			out[id] = a.lastAny
		}
	}
	return out
}

// BuildTemplateArgMap builds keys for {{step_<id>}}, {{prev_<id>}}, {{user_input}}, {{last}}
// and compatibility keys like {{steps.<id>.<field>}} when a step summary is JSON object text.
func BuildTemplateArgMap(inv InvocationContext) map[string]any {
	out := map[string]any{}
	if s := strings.TrimSpace(inv.UserInput); s != "" {
		out["user_input"] = s
	}
	sum := StepSummariesByStepID(inv.Trace)
	for id, txt := range sum {
		if id == "" || txt == "" {
			continue
		}
		out["step_"+id] = txt
		out["prev_"+id] = txt
		// Backward compatibility for templates like {{steps.1.stdout}}.
		out["steps."+id] = txt
		var obj map[string]any
		if err := json.Unmarshal([]byte(txt), &obj); err == nil {
			for k, v := range obj {
				if strings.TrimSpace(k) == "" {
					continue
				}
				out["steps."+id+"."+k] = v
			}
		}
	}
	if len(inv.Trace) > 0 {
		last := inv.Trace[len(inv.Trace)-1]
		if last.Summary != "" {
			out["last"] = last.Summary
			if last.StepID != "" {
				out["last_step_id"] = last.StepID
			}
		}
	}
	return out
}

// BuildInvocationArgs merges template keys, param defaults, FromStepID, user-text heuristics, and required checks.
func BuildInvocationArgs(inv InvocationContext, params []SkillParamSpec) (map[string]any, error) {
	args := map[string]any{}
	for k, v := range BuildTemplateArgMap(inv) {
		args[k] = v
	}
	stepSum := StepSummariesByStepID(inv.Trace)

	for _, p := range params {
		if p.Name == "" {
			continue
		}
		if p.Default != nil {
			if _, ok := args[p.Name]; !ok {
				args[p.Name] = p.Default
			}
		}
	}
	for _, p := range params {
		if p.Name == "" || args[p.Name] != nil {
			continue
		}
		if p.FromStepID != "" {
			if s, ok := stepSum[p.FromStepID]; ok && strings.TrimSpace(s) != "" {
				args[p.Name] = s
			}
		}
	}
	for _, p := range params {
		if p.Name == "" || args[p.Name] != nil {
			continue
		}
		switch strings.ToLower(p.Name) {
		case "query", "text", "prompt", "message":
			if strings.TrimSpace(inv.UserInput) != "" {
				args[p.Name] = inv.UserInput
			}
		}
	}
	for _, p := range params {
		if !p.Required || strings.TrimSpace(p.Name) == "" {
			continue
		}
		if args[p.Name] == nil {
			return nil, fmt.Errorf("missing required skill arg %q", p.Name)
		}
	}
	return args, nil
}

// InvokeSkillVariant builds args and instantiates the variant plan (single entry for skill routing).
func InvokeSkillVariant(v *SkillPlanVariant, inv InvocationContext) (*ports.Plan, error) {
	if v == nil || v.Plan == nil {
		return nil, fmt.Errorf("skill.InvokeSkillVariant: nil variant or plan")
	}
	args, err := BuildInvocationArgs(inv, v.Params)
	if err != nil {
		return nil, err
	}
	return InstantiateSkillPlan(v.Plan, args), nil
}

// RenderPlanStepArguments clones a plan step's arguments and substitutes {{keys}} using current task trace and user input.
// Use before MCP Invoke so templates can reference prior steps ({{step_1}}, {{last}}, etc.).
func RenderPlanStepArguments(taskState *ports.Task, stepID string) map[string]any {
	if taskState == nil || taskState.Plan == nil {
		return nil
	}
	for i := range taskState.Plan.Steps {
		if taskState.Plan.Steps[i].ID != stepID {
			continue
		}
		st := taskState.Plan.Steps[i]
		if len(st.Arguments) == 0 {
			return nil
		}
		inv := InvocationContext{UserInput: taskState.UserInput.Text, Trace: taskState.Trace, Plan: taskState.Plan}
		tmpl := BuildTemplateArgMap(inv)
		return renderArgumentMap(maps.Clone(st.Arguments), tmpl)
	}
	return nil
}
