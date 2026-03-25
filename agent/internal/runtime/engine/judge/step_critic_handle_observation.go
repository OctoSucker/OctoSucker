package judge

import (
	"context"
	"fmt"

	"github.com/OctoSucker/agent/pkg/ports"
	rtutils "github.com/OctoSucker/agent/utils"
)

func (x *StepCritic) HandleObservationReady(ctx context.Context, evt ports.Event) (*ports.Event, error) {
	pl := evt.Payload.(ports.PayloadObservation)
	obs := pl.Obs
	task, ok := x.Tasks.Get(pl.TaskID)
	if !ok {
		return nil, fmt.Errorf("step_critic: task %q not found", pl.TaskID)
	}
	task.StepID = pl.StepID
	task.Trace = append(task.Trace, ports.StepTrace{StepID: pl.StepID, Tool: pl.Tool, OK: obs.Err == nil, Summary: obs.Summary})
	if obs.Err != nil {
		msg := obs.Err.Error()
		if msg == "" {
			msg = obs.Summary
		}
		if err := x.NodeFailures.RecordFailure(pl.Capability, pl.Tool, task.LastCapability, msg); err != nil {
			return nil, fmt.Errorf("step_critic: RecordFailure: %w", err)
		}
	}
	if obs.Err == nil && task.ToolFailCount != nil {
		delete(task.ToolFailCount, rtutils.ToolFailCountKey(pl.StepID, pl.Tool))
	}
	if obs.Err == nil && task.CapabilityFailCount != nil {
		delete(task.CapabilityFailCount, rtutils.CapabilityFailCountKey(pl.StepID, pl.Capability))
	}
	if pl.StepID == task.CapChainStepID && len(task.CapChainTools) > 1 {
		if obs.Err != nil {
			return x.handleCapChainToolFailure(ctx, task, pl)
		}
		task.CapChainNext++
		if task.CapChainNext < len(task.CapChainTools) {
			task.LastStepDecision = &ports.Decision{Action: ports.ActionAccept, Reason: "next in chain"}
			task.PendingTool = task.CapChainTools[task.CapChainNext]
			if err := x.Tasks.Put(task); err != nil {
				return nil, err
			}
			return ports.EventPtr(ports.Event{Type: ports.EvToolCall, Payload: ports.PayloadToolCall{
				TaskID: pl.TaskID, StepID: pl.StepID, Capability: pl.Capability, Tool: task.PendingTool, Arguments: planStepArgsForTool(task, pl.StepID),
			}}), nil
		}
		task.LastStepDecision = &ports.Decision{Action: ports.ActionAccept, Reason: "chain done"}
		task.CapChainStepID, task.CapChainTools, task.CapChainNext = "", nil, 0
	} else if obs.Err != nil && len(task.CapChainTools) <= 1 {
		ev, err := x.handleSingleToolFailure(ctx, task, pl)
		if err != nil || ev != nil {
			return ev, err
		}
	}
	outcome := 0
	if obs.Err != nil {
		outcome = 1
	}
	if task.LastStepDecision == nil {
		task.LastStepDecision = &ports.Decision{Action: ports.ActionAccept, Reason: "step done"}
	}
	if x.RouteGraph != nil {
		if err := x.RouteGraph.RecordTransition(ctx, ports.RoutingContext{IntentText: task.UserInput.Text}, task.LastCapability, pl.Capability, outcome); err != nil {
			return nil, fmt.Errorf("step_critic: RecordTransition: %w", err)
		}
		task.TransitionPath = append(task.TransitionPath, ports.TransitionStep{From: task.LastCapability, To: pl.Capability})
	}
	task.Plan.MarkDone(pl.StepID)
	task.LastCapability, task.LastOutcome = pl.Capability, outcome
	if err := x.Tasks.Put(task); err != nil {
		return nil, err
	}
	return ports.EventPtr(ports.Event{Type: ports.EvPlanProgressed, Payload: ports.PayloadPlanProgressed{TaskID: pl.TaskID}}), nil
}
