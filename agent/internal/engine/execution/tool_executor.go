package execution

import (
	"context"
	"log"

	"github.com/OctoSucker/agent/pkg/ports"
	skillsbuiltin "github.com/OctoSucker/agent/repo/capability/builtin/skills"
	routinggraph "github.com/OctoSucker/agent/repo/routing_graph"
	"github.com/OctoSucker/agent/repo/task"
)

type ToolExecutor struct {
	Tasks            *task.TaskStore
	RouteGraph       *routinggraph.RoutingGraph
	OnCatalogChanged func(context.Context) error
}

func (x *ToolExecutor) HandleToolCall(ctx context.Context, evt ports.Event) (*ports.Event, error) {
	pl := evt.Payload.(ports.PayloadToolCall)
	res, err := x.RouteGraph.Invoke(ctx, ports.CapabilityInvocation{
		CapabilityName: pl.Capability,
		Tool:           pl.Tool,
		Arguments:      pl.Arguments,
	})
	if err != nil {
		res = ports.ToolResult{OK: false, Err: err}
	} else {
		if res.OK && pl.Capability == skillsbuiltin.CapabilityName &&
			pl.Tool == skillsbuiltin.ToolReloadSkills &&
			x.OnCatalogChanged != nil {
			if syncErr := x.OnCatalogChanged(ctx); syncErr != nil {
				log.Printf("tool_executor: OnCatalogChanged after skills reload: %v", syncErr)
			}
		}
	}

	obs := res.Observation()
	log.Printf(
		"tool_executor: invoke done task=%s step=%s capability=%s tool=%s arguments=%v ok=%v err=%v summary=%q, structured=%v",
		pl.TaskID, pl.StepID, pl.Capability, pl.Tool, pl.Arguments, obs.Err == nil, obs.Err, obs.Summary, obs.Structured,
	)
	return ports.EventPtr(ports.Event{Type: ports.EvObservationReady, Payload: ports.PayloadObservation{
		TaskID: pl.TaskID, StepID: pl.StepID, Capability: pl.Capability, Tool: pl.Tool, Obs: obs,
	}}), nil
}
