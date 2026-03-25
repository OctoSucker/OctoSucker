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
	if len(p.ToolInputSchemas) > 0 {
		for _, st := range plan.Steps {
			schema, ok := p.ToolInputSchemas[st.Capability]
			if !ok {
				return nil, fmt.Errorf("no input schema for capability %q", st.Capability)
			}
			if err := mcpclient.ValidateToolArguments(st.Capability, st.Arguments, schema); err != nil {
				log.Printf("engine.Dispatcher: plan arguments invalid task=%s err=%v", taskID, err)
				return nil, fmt.Errorf("planner: plan tool arguments: step id=%q capability=%q: %w", st.ID, st.Capability, err)
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
