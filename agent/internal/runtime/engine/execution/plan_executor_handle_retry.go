package execution

import (
	"context"
	"fmt"

	"github.com/OctoSucker/agent/pkg/ports"
	rtutils "github.com/OctoSucker/agent/utils"
)

// HandleStepCapabilityRetry switches the step to another capability and restarts that step.
func (x *PlanExecutor) HandleStepCapabilityRetry(ctx context.Context, evt ports.Event) (*ports.Event, error) {
	pl := evt.Payload.(ports.PayloadStepCapabilityRetry)
	task, ok := x.Tasks.Get(pl.TaskID)
	if !ok || task.Plan == nil {
		return nil, fmt.Errorf("plan_executor: StepCapabilityRetry: task %q missing or no plan", pl.TaskID)
	}
	st := rtutils.FindPlanStep(task.Plan, pl.StepID)
	if st == nil {
		return nil, fmt.Errorf("plan_executor: StepCapabilityRetry: step %q not in plan", pl.StepID)
	}
	snap, err := task.RouteSnap()
	if err != nil {
		return nil, err
	}
	capID, err := x.resolvePlanCapability(ctx, snap, st.Capability, pl.ExcludeCapability)
	if err != nil {
		return nil, err
	}
	if capID == "" {
		return nil, fmt.Errorf("plan_executor: no alternative capability for step %q", pl.StepID)
	}
	st.Capability = capID
	task.CapChainStepID, task.CapChainTools, task.CapChainNext, task.StepID, task.PendingTool = "", nil, 0, "", ""
	return x.startPlanStep(ctx, task, pl.StepID)
}
