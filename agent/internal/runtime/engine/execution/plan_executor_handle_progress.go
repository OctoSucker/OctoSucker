package execution

import (
	"context"
	"fmt"

	"github.com/OctoSucker/agent/pkg/ports"
)

// HandlePlanProgressed runs the next runnable step(s) or emits trajectory check when the plan is done.
func (x *PlanExecutor) HandlePlanProgressed(ctx context.Context, evt ports.Event) (*ports.Event, error) {
	pl := evt.Payload.(ports.PayloadPlanProgressed)

	task, ok := x.Tasks.Get(pl.TaskID)
	if !ok || task.Plan == nil {
		return nil, fmt.Errorf("plan_executor: session %q missing or no plan", pl.TaskID)
	}
	runnable := task.Plan.Runnable()
	if len(runnable) == 0 {
		if task.Plan.AllDone() {
			return ports.EventPtr(ports.Event{Type: ports.EvTrajectoryCheck, Payload: ports.PayloadTrajectoryCheck{TaskID: pl.TaskID}}), nil
		}
		return nil, fmt.Errorf("plan_executor: session %q has no runnable steps but plan is not all done", pl.TaskID)
	}
	// Agent runtime uses single-step scheduling per turn; subsequent steps continue via EvPlanProgressed loop.
	return x.startPlanStep(ctx, task, runnable[0].ID)
}
