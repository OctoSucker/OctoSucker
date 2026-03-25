package judge

import (
	"context"

	skill "github.com/OctoSucker/agent/internal/runtime/store/skill"
	"github.com/OctoSucker/agent/pkg/ports"
	rtutils "github.com/OctoSucker/agent/utils"
)

// handleCapChainToolFailure runs when a tool in a multi-tool capability chain fails.
// Always returns a non-nil event or an error (caller returns immediately).
func (x *StepCritic) handleCapChainToolFailure(ctx context.Context, taskState *ports.Task, pl ports.PayloadObservation) (*ports.Event, error) {
	nCap := x.bumpCapabilityFailCount(taskState, pl.StepID, pl.Capability)
	if nCap >= x.maxCapabilityFails() {
		alt, err := x.trySwitchCapability(ctx, taskState, pl.Capability)
		if err != nil {
			return nil, err
		}
		if alt != "" {
			outcome := 1
			if x.RouteGraph != nil {
				if err := x.RouteGraph.RecordTransition(ctx, ports.RoutingContext{IntentText: taskState.UserInput.Text}, taskState.LastCapability, pl.Capability, outcome); err != nil {
					return nil, err
				}
			}
			taskState.TransitionPath = append(taskState.TransitionPath, ports.TransitionStep{From: taskState.LastCapability, To: pl.Capability})
			taskState.LastStepDecision = &ports.Decision{Action: ports.ActionSwitchCapability, Reason: "capability_fail_threshold, switch"}
			taskState.CapChainStepID, taskState.CapChainTools, taskState.CapChainNext = "", nil, 0
			taskState.LastCapability, taskState.LastOutcome = pl.Capability, outcome
			taskState.Plan.MarkPending(pl.StepID)
			if err := x.Tasks.Put(taskState); err != nil {
				return nil, err
			}
			return ports.EventPtr(ports.Event{Type: ports.EvStepCapabilityRetry, Payload: ports.PayloadStepCapabilityRetry{TaskID: pl.TaskID, StepID: pl.StepID, ExcludeCapability: pl.Capability}}), nil
		}
	}
	if x.shouldRetryTool(taskState, pl) {
		taskState.LastStepDecision = &ports.Decision{Action: ports.ActionRetry, Reason: "tool failed, retry"}
		if err := x.Tasks.Put(taskState); err != nil {
			return nil, err
		}
		return ports.EventPtr(ports.Event{Type: ports.EvToolCall, Payload: ports.PayloadToolCall{TaskID: pl.TaskID, StepID: pl.StepID, Capability: pl.Capability, Tool: pl.Tool, Arguments: planStepArgsForTool(taskState, pl.StepID)}}), nil
	}
	outcome := 1
	if x.RouteGraph != nil {
		if err := x.RouteGraph.RecordTransition(ctx, ports.RoutingContext{IntentText: taskState.UserInput.Text}, taskState.LastCapability, pl.Capability, outcome); err != nil {
			return nil, err
		}
	}
	taskState.TransitionPath = append(taskState.TransitionPath, ports.TransitionStep{From: taskState.LastCapability, To: pl.Capability})
	alt, err := x.trySwitchCapability(ctx, taskState, pl.Capability)
	if err != nil {
		return nil, err
	}
	if alt != "" {
		taskState.LastStepDecision = &ports.Decision{Action: ports.ActionSwitchCapability, Reason: "cap chain fail, try other capability"}
		taskState.CapChainStepID, taskState.CapChainTools, taskState.CapChainNext = "", nil, 0
		taskState.LastCapability, taskState.LastOutcome = pl.Capability, outcome
		taskState.Plan.MarkPending(pl.StepID)
		if err := x.Tasks.Put(taskState); err != nil {
			return nil, err
		}
		return ports.EventPtr(ports.Event{Type: ports.EvStepCapabilityRetry, Payload: ports.PayloadStepCapabilityRetry{TaskID: pl.TaskID, StepID: pl.StepID, ExcludeCapability: pl.Capability}}), nil
	}
	taskState.LastStepDecision = &ports.Decision{Action: ports.ActionAccept, Reason: "cap chain fail, mark done"}
	taskState.CapChainStepID, taskState.CapChainTools, taskState.CapChainNext = "", nil, 0
	taskState.LastCapability, taskState.LastOutcome = pl.Capability, outcome
	taskState.Plan.MarkDone(pl.StepID)
	if err := x.Tasks.Put(taskState); err != nil {
		return nil, err
	}
	return ports.EventPtr(ports.Event{Type: ports.EvPlanProgressed, Payload: ports.PayloadPlanProgressed{TaskID: pl.TaskID}}), nil
}

// handleSingleToolFailure runs when the step uses a single tool (or no multi-tool chain) and the call failed.
// Returns (nil, nil) only when the caller should continue to the common step-completion path (mark done + graph).
func (x *StepCritic) handleSingleToolFailure(ctx context.Context, taskState *ports.Task, pl ports.PayloadObservation) (*ports.Event, error) {
	nCap := x.bumpCapabilityFailCount(taskState, pl.StepID, pl.Capability)
	if nCap >= x.maxCapabilityFails() {
		alt, err := x.trySwitchCapability(ctx, taskState, pl.Capability)
		if err != nil {
			return nil, err
		}
		if alt != "" {
			taskState.LastStepDecision = &ports.Decision{Action: ports.ActionSwitchCapability, Reason: "capability_fail_threshold, switch"}
			taskState.LastCapability, taskState.LastOutcome = pl.Capability, 1
			taskState.Plan.MarkPending(pl.StepID)
			if err := x.Tasks.Put(taskState); err != nil {
				return nil, err
			}
			return ports.EventPtr(ports.Event{Type: ports.EvStepCapabilityRetry, Payload: ports.PayloadStepCapabilityRetry{TaskID: pl.TaskID, StepID: pl.StepID, ExcludeCapability: pl.Capability}}), nil
		}
	}
	if x.shouldRetryTool(taskState, pl) {
		taskState.LastStepDecision = &ports.Decision{Action: ports.ActionRetry, Reason: "tool failed, retry"}
		if err := x.Tasks.Put(taskState); err != nil {
			return nil, err
		}
		return ports.EventPtr(ports.Event{Type: ports.EvToolCall, Payload: ports.PayloadToolCall{TaskID: pl.TaskID, StepID: pl.StepID, Capability: pl.Capability, Tool: pl.Tool, Arguments: planStepArgsForTool(taskState, pl.StepID)}}), nil
	}
	alt, err := x.trySwitchCapability(ctx, taskState, pl.Capability)
	if err != nil {
		return nil, err
	}
	if alt != "" {
		taskState.LastStepDecision = &ports.Decision{Action: ports.ActionSwitchCapability, Reason: "single tool fail, try other capability"}
		taskState.LastCapability, taskState.LastOutcome = pl.Capability, 1
		taskState.Plan.MarkPending(pl.StepID)
		if err := x.Tasks.Put(taskState); err != nil {
			return nil, err
		}
		return ports.EventPtr(ports.Event{Type: ports.EvStepCapabilityRetry, Payload: ports.PayloadStepCapabilityRetry{TaskID: pl.TaskID, StepID: pl.StepID, ExcludeCapability: pl.Capability}}), nil
	}
	taskState.LastStepDecision = &ports.Decision{Action: ports.ActionAccept, Reason: "single tool fail, mark done"}
	return nil, nil
}

func (x *StepCritic) shouldRetryTool(task *ports.Task, pl ports.PayloadObservation) bool {
	if pl.Obs.Err == nil {
		return false
	}
	if task.ToolFailCount == nil {
		task.ToolFailCount = make(map[string]int)
	}
	k := rtutils.ToolFailCountKey(pl.StepID, pl.Tool)
	task.ToolFailCount[k]++
	return task.ToolFailCount[k] < x.maxStepToolFails()
}

func planStepArgsForTool(taskState *ports.Task, stepID string) map[string]any {
	a := skill.RenderPlanStepArguments(taskState, stepID)
	if a == nil {
		return rtutils.PlanStepArguments(taskState, stepID)
	}
	return a
}
