package planning

import (
	"fmt"

	"github.com/OctoSucker/agent/pkg/ports"
	"github.com/google/uuid"
)

func reassignPlanStepIDsWithUUID(plan *ports.Plan) (*ports.Plan, error) {
	if plan == nil || len(plan.Steps) == 0 {
		return nil, fmt.Errorf("planner: reassign step ids: empty plan")
	}
	idMap := make(map[string]string, len(plan.Steps))
	for _, st := range plan.Steps {
		oldID := st.ID
		if oldID == "" {
			return nil, fmt.Errorf("planner: reassign step ids: empty step id")
		}
		if _, exists := idMap[oldID]; exists {
			return nil, fmt.Errorf("planner: reassign step ids: duplicate step id %q", oldID)
		}
		idMap[oldID] = uuid.NewString()
	}
	out := &ports.Plan{Steps: make([]ports.PlanStep, len(plan.Steps))}
	for i := range plan.Steps {
		st := plan.Steps[i].Clone()
		st.ID = idMap[st.ID]
		deps := make([]string, 0, len(st.DependsOn))
		for _, dep := range st.DependsOn {
			mappedDep, ok := idMap[dep]
			if !ok {
				return nil, fmt.Errorf("planner: reassign step ids: unresolved dependency %q", dep)
			}
			deps = append(deps, mappedDep)
		}
		st.DependsOn = deps
		out.Steps[i] = st
	}
	return out, nil
}
