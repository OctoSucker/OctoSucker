package plantemplate

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/OctoSucker/agent/pkg/ports"
)

// RenderPlanStepArguments substitutes {{placeholders}} in the step's argument map (recursively through nested maps and slices).
func RenderPlanStepArguments(task *ports.Task, stepID string) (map[string]any, error) {
	st := task.Plan.FindStep(stepID)
	if len(st.Arguments) == 0 {
		return map[string]any{}, nil
	}
	ctx, err := buildTemplateArgMap(task.UserInput, task.Plan)
	if err != nil {
		return nil, err
	}
	out := make(map[string]any, len(st.Arguments))
	for k, v := range st.Arguments {
		out[k] = applyArgTemplates(v, ctx)
	}
	return out, nil
}

func buildTemplateArgMap(userInput string, plan *ports.Plan) (map[string]any, error) {
	out := map[string]any{}
	if s := strings.TrimSpace(userInput); s != "" {
		out["user_input"] = s
	}
	sum, err := ports.StepSummariesFromPlan(plan)
	if err != nil {
		return nil, err
	}
	for id, txt := range sum {
		if id == "" || txt == "" {
			continue
		}
		out["step_"+id] = txt
		out["prev_"+id] = txt
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
	lastID, lastText, err := ports.LastDoneStepPrimary(plan)
	if err != nil {
		return nil, err
	}
	if lastText != "" {
		out["last"] = lastText
		if lastID != "" {
			out["last_step_id"] = lastID
		}
	}
	return out, nil
}

func applyArgTemplates(v any, ctx map[string]any) any {
	switch t := v.(type) {
	case string:
		return substituteStringTemplate(t, ctx)
	case []any:
		out := make([]any, len(t))
		for i := range t {
			out[i] = applyArgTemplates(t[i], ctx)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, v := range t {
			out[k] = applyArgTemplates(v, ctx)
		}
		return out
	default:
		return v
	}
}

func substituteStringTemplate(s string, ctx map[string]any) any {
	trimmed := strings.TrimSpace(s)
	if strings.HasPrefix(trimmed, "{{") && strings.HasSuffix(trimmed, "}}") && strings.Count(trimmed, "{{") == 1 && strings.Count(trimmed, "}}") == 1 {
		name := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(trimmed, "{{"), "}}"))
		if val, ok := ctx[name]; ok {
			return val
		}
		return s
	}
	out := s
	for k, v := range ctx {
		if k == "" {
			continue
		}
		out = strings.ReplaceAll(out, "{{"+k+"}}", fmt.Sprint(v))
	}
	return out
}
