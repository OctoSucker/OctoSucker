package planning

import (
	"fmt"
	"log"

	"github.com/OctoSucker/agent/pkg/mcpclient"
	"github.com/OctoSucker/agent/pkg/ports"
)

func (p *Planner) finalizePlan(taskID string, taskState *ports.Task, plan *ports.Plan) (*ports.Event, error) {
	if plan == nil || len(plan.Steps) == 0 {
		return nil, fmt.Errorf("planner: empty plan")
	}
	if p.CapRegistry == nil {
		return nil, fmt.Errorf("planner: nil CapRegistry")
	}
	schemas := p.CapRegistry.ToolInputSchemasByName()
	if len(schemas) > 0 {
		for _, st := range plan.Steps {
			ok := p.CapRegistry.CheckStepTool(st.Capability, st.Tool)
			if !ok {
				return nil, fmt.Errorf("planner: step id=%v", st.ID)
			}
			schema, ok := schemas[st.Tool]
			if !ok {
				return nil, fmt.Errorf("no input schema for tool %q (capability %q)", st.Tool, st.Capability)
			}
			if err := mcpclient.ValidateToolArguments(st.Tool, st.Arguments, schema); err != nil {
				log.Printf("engine.Dispatcher: plan arguments invalid task=%s err=%v", taskID, err)
				return nil, fmt.Errorf("planner: plan tool arguments: step id=%q capability=%q tool=%q: %w", st.ID, st.Capability, st.Tool, err)
			}
		}
	}
	for i := range plan.Steps {
		if plan.Steps[i].Status == "" {
			plan.Steps[i].Status = "pending"
		}
	}
	taskState.Plan = plan
	taskState.LastCapability = ""
	taskState.LastOutcome = 0
	if err := p.Tasks.Put(taskState); err != nil {
		return nil, err
	}
	return ports.EventPtr(ports.Event{Type: ports.EvPlanProgressed, Payload: ports.PayloadPlanProgressed{TaskID: taskID}}), nil
}
