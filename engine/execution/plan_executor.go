package execution

import (
	"context"
	"fmt"
	"log"
	"strconv"

	"github.com/OctoSucker/octosucker/engine/types"
	"github.com/OctoSucker/octosucker/repo/taskstore"
	"github.com/OctoSucker/octosucker/repo/toolprovider"
)

// PlanExecutor advances the plan by running the next runnable tool synchronously and emitting EvObservationReady.
type PlanExecutor struct {
	Tasks        *taskstore.TaskStore
	ToolRegistry *toolprovider.Registry
}

// HandlePlanProgressed runs the next pending step (mark running, invoke tool, enqueue observation). One planner turn should add at least one step; trajectory runs after each observation, not via AllDone here.
func (x *PlanExecutor) HandlePlanProgressed(ctx context.Context, pl types.PayloadPlanProgressed) (*types.Event, error) {
	task, ok := x.Tasks.Get(pl.TaskID)
	if !ok || task.Plan == nil {
		return nil, fmt.Errorf("plan_executor: task %q missing or no plan", pl.TaskID)
	}
	runnable := task.Plan.Runnable()
	if len(runnable) == 0 {
		if task.Plan.AllDone() {
			return nil, fmt.Errorf("plan_executor: task %q has no pending step but all steps are done (state machine error; trajectory should follow the last observation)", pl.TaskID)
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
	argMap, err := task.RenderPlanStepArguments(step.ID)
	if err != nil {
		return nil, fmt.Errorf("plan_executor: render step arguments: %w", err)
	}

	res, err := x.ToolRegistry.Invoke(ctx, step.Node.Tool, argMap)
	if err != nil {
		res = types.ToolResult{Err: err}
	}

	log.Printf(
		"plan_executor: task=%s step=%s tool=%s args=%v %s",
		pl.TaskID, step.ID, step.Node.Tool, argMap, summarizeToolResultForLog(res),
	)
	return types.EventPtr(types.Event{Type: types.EvObservationReady, Payload: types.PayloadObservation{
		TaskID: pl.TaskID, StepID: step.ID, Result: res,
	}}), nil
}

const toolResultLogMax = 480

func summarizeToolResultForLog(res types.ToolResult) string {
	if res.Err != nil {
		return "err=" + res.Err.Error()
	}
	s := (&res).CompactForLLM()
	if len(s) <= toolResultLogMax {
		return "ok " + s
	}
	return "ok (compact_len=" + strconv.Itoa(len(s)) + ") " + s[:toolResultLogMax] + "…"
}
