package execution

import (
	"context"
	"fmt"

	"github.com/OctoSucker/agent/pkg/ports"
	"github.com/OctoSucker/agent/repo/plantemplate"
)

// HandlePlanProgressed runs when Task.Plan is non-empty (dispatcher sends here after a plan exists). Empty-plan synthesis is not handled here.
func (x *PlanExecutor) HandlePlanProgressed(ctx context.Context, evt ports.Event) (*ports.Event, error) {
	pl := evt.Payload.(ports.PayloadPlanProgressed)
	task, ok := x.Tasks.Get(pl.TaskID)
	if !ok || task.Plan == nil {
		return nil, fmt.Errorf("plan_executor: task %q missing or no plan", pl.TaskID)
	}
	runnable := task.Plan.Runnable()
	if len(runnable) == 0 {
		if task.Plan.AllDone() {
			return ports.EventPtr(ports.Event{Type: ports.EvTrajectoryCheck, Payload: ports.PayloadTrajectoryCheck{TaskID: pl.TaskID}}), nil
		}
		return nil, fmt.Errorf("plan_executor: task %q has no runnable steps but plan is not all done", pl.TaskID)
	}
	step := task.Plan.FindStep(runnable[0].ID)
	if step == nil {
		return nil, fmt.Errorf("plan_executor: step %q not found in plan", runnable[0].ID)
	}
	task.Plan.MarkRunning(step.ID)

	if err := x.Tasks.Put(task); err != nil {
		return nil, err
	}
	argMap := plantemplate.RenderPlanStepArguments(task, step.ID)
	return ports.EventPtr(ports.Event{Type: ports.EvToolCall, Payload: ports.PayloadToolCall{
		TaskID: task.ID, StepID: step.ID, Capability: step.Capability, Tool: step.Tool, Arguments: argMap,
	}}), nil
}
