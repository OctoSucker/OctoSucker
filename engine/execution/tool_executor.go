package execution

import (
	"context"
	"log"

	"github.com/OctoSucker/octosucker/engine/types"
	"github.com/OctoSucker/octosucker/repo/toolprovider"
	"github.com/OctoSucker/octosucker/repo/taskstore"
)

type ToolExecutor struct {
	Tasks        *taskstore.TaskStore
	ToolRegistry *toolprovider.Registry
}

func (x *ToolExecutor) HandleToolCall(ctx context.Context, pl types.PayloadToolCall) (*types.Event, error) {
	res, err := x.ToolRegistry.Invoke(ctx, pl.Node.Tool, pl.Arguments)
	if err != nil {
		res = types.ToolResult{Err: err}
	}

	log.Printf(
		"tool_executor: invoke done task=%s step=%s node=%s arguments=%v result=%v",
		pl.TaskID, pl.StepID, pl.Node.String(), pl.Arguments, res,
	)
	return types.EventPtr(types.Event{Type: types.EvObservationReady, Payload: types.PayloadObservation{
		TaskID: pl.TaskID, StepID: pl.StepID, Result: res,
	}}), nil
}
