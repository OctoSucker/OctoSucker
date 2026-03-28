package argtmpl

import (
	"fmt"
	"maps"
	"strings"

	procedure "github.com/OctoSucker/agent/internal/store/procedure"
	"github.com/OctoSucker/agent/pkg/ports"
)

// RenderPlanStepArguments clones a plan step's arguments and substitutes {{keys}} using current task trace and user input.
func RenderPlanStepArguments(taskState *ports.Task, stepID string) map[string]any {
	if taskState == nil || taskState.Plan == nil {
		return nil
	}
	st := taskState.Plan.FindStep(stepID)
	if st == nil || len(st.Arguments) == 0 {
		return nil
	}
	inv := procedure.InvocationContext{UserInput: taskState.UserInput.Text, Trace: taskState.Trace, Plan: taskState.Plan}
	tmpl := procedure.BuildTemplateArgMap(inv)
	return renderArgumentMap(maps.Clone(st.Arguments), tmpl)
}

func renderArgumentMap(in map[string]any, args map[string]any) map[string]any {
	if len(in) == 0 {
		return in
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = renderAny(v, args)
	}
	return out
}

func renderAny(v any, args map[string]any) any {
	switch t := v.(type) {
	case string:
		return renderStringTemplate(t, args)
	case []any:
		out := make([]any, len(t))
		for i := range t {
			out[i] = renderAny(t[i], args)
		}
		return out
	case map[string]any:
		return renderArgumentMap(t, args)
	default:
		return v
	}
}

func renderStringTemplate(s string, args map[string]any) any {
	trimmed := strings.TrimSpace(s)
	if strings.HasPrefix(trimmed, "{{") && strings.HasSuffix(trimmed, "}}") && strings.Count(trimmed, "{{") == 1 && strings.Count(trimmed, "}}") == 1 {
		name := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(trimmed, "{{"), "}}"))
		if val, ok := args[name]; ok {
			return val
		}
		return s
	}
	out := s
	for k, v := range args {
		if k == "" {
			continue
		}
		out = strings.ReplaceAll(out, "{{"+k+"}}", fmt.Sprint(v))
	}
	return out
}
