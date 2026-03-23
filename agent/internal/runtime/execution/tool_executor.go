package execution

import (
	"context"
	"maps"

	"github.com/OctoSucker/agent/pkg/ports"
)

type SessionRepository interface {
	Get(id string) (*ports.Session, bool)
}

type ToolInvoker interface {
	Invoke(ctx context.Context, inv ports.CapabilityInvocation) (ports.ToolResult, error)
}

type ToolExecutor struct {
	Sessions SessionRepository
	Invoker  ToolInvoker
}

func (x *ToolExecutor) HandleToolCall(ctx context.Context, evt ports.Event) ([]ports.Event, error) {
	pl := evt.Payload.(ports.PayloadToolCall)
	if _, ok := x.Sessions.Get(pl.SessionID); !ok {
		return nil, nil
	}
	res, err := x.Invoker.Invoke(ctx, ports.CapabilityInvocation{
		CapabilityID: pl.Capability,
		Tool:         pl.Tool,
		Arguments:    maps.Clone(pl.Arguments),
	})
	if err != nil {
		res = ports.ToolResult{OK: false, Err: err}
	}
	obs := res.Observation()
	return []ports.Event{{Type: ports.EvObservationReady, Payload: ports.PayloadObservation{
		SessionID: pl.SessionID, StepID: pl.StepID, Capability: pl.Capability, Tool: pl.Tool, Obs: obs,
	}}}, nil
}
