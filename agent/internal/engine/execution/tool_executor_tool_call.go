package execution

import (
	"context"
	"log"

	"github.com/OctoSucker/agent/internal/store/capability"
	skillsbuiltin "github.com/OctoSucker/agent/internal/store/capability/builtin/skills"
	"github.com/OctoSucker/agent/internal/store/task"
	"github.com/OctoSucker/agent/pkg/ports"
)

type ToolExecutor struct {
	Tasks            *task.TaskStore
	Invoker          *capability.CapabilityRegistry
	OnCatalogChanged func(context.Context) error
}

func (x *ToolExecutor) HandleToolCall(ctx context.Context, evt ports.Event) (*ports.Event, error) {
	pl := evt.Payload.(ports.PayloadToolCall)
	if _, ok := x.Tasks.Get(pl.TaskID); !ok {
		return nil, nil
	}
	log.Printf(
		"tool_executor: invoke start task=%s step=%s capability=%s tool=%s args=%v",
		pl.TaskID, pl.StepID, pl.Capability, pl.Tool, pl.Arguments,
	)
	res, err := x.Invoker.Invoke(ctx, ports.CapabilityInvocation{
		CapabilityName: pl.Capability,
		Tool:           pl.Tool,
		Arguments:      pl.Arguments,
	})
	if err != nil {
		res = ports.ToolResult{OK: false, Err: err}
	}
	if err == nil && res.OK && pl.Capability == skillsbuiltin.CapabilityName && pl.Tool == skillsbuiltin.ToolReloadSkills && x.OnCatalogChanged != nil {
		if syncErr := x.OnCatalogChanged(ctx); syncErr != nil {
			log.Printf("tool_executor: OnCatalogChanged after skills reload: %v", syncErr)
		}
	}
	obs := res.Observation()
	log.Printf(
		"tool_executor: invoke done task=%s step=%s capability=%s tool=%s ok=%v err=%v summary=%q, structured=%v",
		pl.TaskID, pl.StepID, pl.Capability, pl.Tool, obs.Err == nil, obs.Err, obs.Summary, obs.Structured,
	)
	return ports.EventPtr(ports.Event{Type: ports.EvObservationReady, Payload: ports.PayloadObservation{
		TaskID: pl.TaskID, StepID: pl.StepID, Capability: pl.Capability, Tool: pl.Tool, Obs: obs,
	}}), nil
}
