package engine

import (
	"context"
	"log"
	"maps"

	"github.com/OctoSucker/agent/pkg/ports"
)

func (d *Dispatcher) toolExecutorToolCall(ctx context.Context, evt ports.Event) ([]ports.Event, error) {
	pl := evt.Payload.(ports.PayloadToolCall)
	if _, ok := d.Sessions.Get(pl.SessionID); !ok {
		return nil, nil
	}
	res, err := d.MCPRouter.Invoke(ctx, ports.CapabilityInvocation{
		CapabilityID: pl.Capability,
		Tool:         pl.Tool,
		Arguments:    maps.Clone(pl.Arguments),
	})
	if err != nil {
		log.Printf("engine.Dispatcher: MCPRouter.Invoke error session=%s step=%s capability=%s tool=%q arguments=%v err=%v",
			pl.SessionID, pl.StepID, pl.Capability, pl.Tool, pl.Arguments, err)
		res = ports.ToolResult{OK: false, Err: err}
	}
	obs := res.Observation()
	if obs.Err != nil && err == nil {
		log.Printf("engine.Dispatcher: tool result error session=%s step=%s capability=%s tool=%q summary=%q err=%v",
			pl.SessionID, pl.StepID, pl.Capability, pl.Tool, obs.Summary, obs.Err)
	}
	return []ports.Event{{Type: ports.EvObservationReady, Payload: ports.PayloadObservation{
		SessionID: pl.SessionID, StepID: pl.StepID, Capability: pl.Capability, Tool: pl.Tool, Obs: obs,
	}}}, nil
}
