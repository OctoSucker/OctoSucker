package execution

import (
	"context"
	"log"

	"github.com/OctoSucker/agent/pkg/ports"
	skillsbuiltin "github.com/OctoSucker/agent/repo/tools/builtin/skills"
	routinggraph "github.com/OctoSucker/agent/repo/routing_graph"
	"github.com/OctoSucker/agent/repo/task"
)

type ToolExecutor struct {
	Tasks            *task.TaskStore
	RouteGraph       *routinggraph.RoutingGraph
	OnCatalogChanged func(context.Context) error
}

func (x *ToolExecutor) HandleToolCall(ctx context.Context, pl ports.PayloadToolCall) (*ports.Event, error) {
	res, err := x.RouteGraph.Invoke(ctx, ports.ToolInvocation{
		Tool:      pl.Node.Tool,
		Arguments: pl.Arguments,
	})
	if err != nil {
		res = ports.ToolResult{Err: err}
	} else {
		if pl.Node.Tool == skillsbuiltin.ToolReloadSkills &&
			x.OnCatalogChanged != nil {
			// retry 2 times
			for i := 0; i < 2; i++ {
				if syncErr := x.OnCatalogChanged(ctx); syncErr != nil {
					log.Printf("tool_executor: OnCatalogChanged after skills reload: %v", syncErr)
					continue
				}
				break
			}
		}
	}

	log.Printf(
		"tool_executor: invoke done task=%s step=%s node=%s arguments=%v result=%v",
		pl.TaskID, pl.StepID, pl.Node.String(), pl.Arguments, res,
	)
	return ports.EventPtr(ports.Event{Type: ports.EvObservationReady, Payload: ports.PayloadObservation{
		TaskID: pl.TaskID, StepID: pl.StepID, Result: res,
	}}), nil
}
