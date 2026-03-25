package execution

import (
	"context"
	"fmt"
	"maps"

	skill "github.com/OctoSucker/agent/internal/runtime/store/skill"
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
	tools := x.CapRegistry.Tools(step.Capability)
	if len(tools) == 0 {
		return nil, fmt.Errorf("plan_executor: capability %q has no tools", step.Capability)
	}
	task.Plan.MarkRunning(step.ID)
	if len(tools) == 1 {
		task.CapChainStepID, task.CapChainTools, task.CapChainNext = "", nil, 0
	} else {
		task.CapChainStepID, task.CapChainTools, task.CapChainNext = step.ID, tools, 0
	}
	task.StepID, task.PendingTool = step.ID, tools[0]
	if err := x.Tasks.Put(task); err != nil {
		return nil, err
	}
	argMap := skill.RenderPlanStepArguments(task, step.ID)
	if argMap == nil {
		argMap = maps.Clone(step.Arguments)
	}
	return ports.EventPtr(ports.Event{Type: ports.EvToolCall, Payload: ports.PayloadToolCall{
		TaskID: task.ID, StepID: step.ID, Capability: step.Capability, Tool: tools[0], Arguments: argMap,
	}}), nil
}
