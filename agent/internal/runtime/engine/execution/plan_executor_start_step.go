package execution

import (
	"context"
	"fmt"
	"maps"

	procedure "github.com/OctoSucker/agent/internal/runtime/store/procedure"
	"github.com/OctoSucker/agent/pkg/ports"
	rtutils "github.com/OctoSucker/agent/utils"
)

func (x *PlanExecutor) startPlanStep(ctx context.Context, task *ports.Task, stepID string) (*ports.Event, error) {
	step := rtutils.FindPlanStep(task.Plan, stepID)
	if step == nil {
		return nil, fmt.Errorf("plan_executor: step %q not found in plan", stepID)
	}
	snap, err := task.RouteSnap()
	if err != nil {
		return nil, err
	}
	capID, err := x.resolvePlanCapability(ctx, snap, step.Capability, "")
	if err != nil {
		return nil, err
	}
	if capID != step.Capability {
		step.Capability = capID
	}
	if !x.CapRegistry.CheckStepTool(step.Capability, step.Tool) {
		return nil, fmt.Errorf("plan_executor: invalid tool %q for capability %q", step.Tool, step.Capability)
	}
	task.PendingTool = step.Tool
	task.Plan.MarkRunning(step.ID)
	task.StepID = step.ID

	if err := x.Tasks.Put(task); err != nil {
		return nil, err
	}
	argMap := procedure.RenderPlanStepArguments(task, step.ID)
	if argMap == nil {
		argMap = maps.Clone(step.Arguments)
	}
	return ports.EventPtr(ports.Event{Type: ports.EvToolCall, Payload: ports.PayloadToolCall{
		TaskID: task.ID, StepID: step.ID, Capability: step.Capability, Tool: task.PendingTool, Arguments: argMap,
	}}), nil
}
