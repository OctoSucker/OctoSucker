package planning

import (
	"fmt"
	"log"

	procedure "github.com/OctoSucker/agent/internal/store/procedure"
	"github.com/OctoSucker/agent/pkg/ports"
)

func (p *Planner) buildPlanFromProcedure(task *ports.Task, hit *procedure.ProcedureEntry) (*ports.Plan, error) {
	v := hit.SelectedVariant()
	if v == nil || v.Plan == nil {
		return nil, fmt.Errorf("planner: procedure %q has no selected executable variant", hit.Name)
	}
	inv := procedure.InvocationContext{UserInput: task.UserInput.Text, Trace: task.Trace}
	plan, err := procedure.InvokeProcedureVariant(v, inv)
	if err != nil {
		log.Printf("planner: procedure invocation failed procedure=%s variant=%s err=%v", hit.Name, v.ID, err)
		return nil, fmt.Errorf("planner: invoke procedure variant: %w", err)
	}
	task.RouteSnap.ProcedurePriorNodes = hit.Path
	task.RouteSnap.ProcedurePreferredNodes = hit.Path
	task.ActiveProcedureName = hit.Name
	task.ActiveProcedureVariantID = hit.SelectedVariantID
	return reassignPlanStepIDsWithUUID(plan)
}
