package judge

import (
	"context"
	"fmt"

	argtmpl "github.com/OctoSucker/agent/internal/engine/argtmpl"
	"github.com/OctoSucker/agent/internal/store/capability"
	"github.com/OctoSucker/agent/internal/store/nodefailure"
	routinggraph "github.com/OctoSucker/agent/internal/store/routing_graph"
	taskstore "github.com/OctoSucker/agent/internal/store/task"
	"github.com/OctoSucker/agent/pkg/ports"
	rtutils "github.com/OctoSucker/agent/utils"
)

type StepCritic struct {
	Tasks                 *taskstore.TaskStore
	RouteGraph            *routinggraph.RoutingGraph
	CapRegistry           *capability.CapabilityRegistry
	NodeFailures          *nodefailure.NodeFailureStats
	MaxFailsPerTool       int
	MaxFailsPerCapability int // per stepID+capability failure count before preferring switch (0 = use default 2)
}

func NewStepCritic(tasks *taskstore.TaskStore, routeGraph *routinggraph.RoutingGraph, capRegistry *capability.CapabilityRegistry, nodeFailures *nodefailure.NodeFailureStats, maxFailsPerTool int, maxFailsPerCapability int) *StepCritic {
	return &StepCritic{Tasks: tasks, RouteGraph: routeGraph, CapRegistry: capRegistry, NodeFailures: nodeFailures, MaxFailsPerTool: maxFailsPerTool, MaxFailsPerCapability: maxFailsPerCapability}
}

func (x *StepCritic) HandleObservationReady(ctx context.Context, evt ports.Event) (*ports.Event, error) {
	pl := evt.Payload.(ports.PayloadObservation)
	obs := pl.Obs
	task, ok := x.Tasks.Get(pl.TaskID)
	if !ok {
		return nil, fmt.Errorf("step_critic: task %q not found", pl.TaskID)
	}
	task.StepID = pl.StepID
	task.Trace = append(task.Trace, ports.StepTrace{
		StepID:     pl.StepID,
		Tool:       pl.Tool,
		OK:         obs.Err == nil,
		Summary:    obs.Summary,
		Structured: obs.Structured,
	})
	if obs.Err != nil {
		msg := obs.Err.Error()
		if msg == "" {
			msg = obs.Summary
		}
		lastCap := ""
		if task.RouteSnap != nil && task.RouteSnap.LastNode != "" {
			c, _, ok := routinggraph.ParseNodeID(task.RouteSnap.LastNode)
			if ok {
				lastCap = c
			}
		}
		if err := x.NodeFailures.RecordFailure(pl.Capability, pl.Tool, lastCap, msg); err != nil {
			return nil, fmt.Errorf("step_critic: RecordFailure: %w", err)
		}
	}
	if task.RouteSnap == nil {
		return nil, fmt.Errorf("step_critic: nil RouteSnap")
	}
	if obs.Err == nil && task.ToolFailCount != nil {
		delete(task.ToolFailCount, rtutils.ToolFailCountKey(pl.StepID, pl.Tool))
	}
	if obs.Err == nil && task.CapabilityFailCount != nil {
		delete(task.CapabilityFailCount, rtutils.CapabilityFailCountKey(pl.StepID, pl.Capability))
	}
	if obs.Err != nil {
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
	currentNode := routinggraph.NodeID(pl.Capability, pl.Tool)
	if x.RouteGraph != nil {
		if err := x.RouteGraph.RecordTransition(ctx, ports.RoutingContext{IntentText: task.UserInput.Text}, task.RouteSnap.LastNode, currentNode, outcome); err != nil {
			return nil, fmt.Errorf("step_critic: RecordTransition: %w", err)
		}
		task.TransitionPath = append(task.TransitionPath, ports.TransitionStep{From: task.RouteSnap.LastNode, To: currentNode})
	}
	task.Plan.MarkDone(pl.StepID)
	task.RouteSnap.LastNode = currentNode
	task.RouteSnap.LastOut = outcome
	if err := x.Tasks.Put(task); err != nil {
		return nil, err
	}
	return ports.EventPtr(ports.Event{Type: ports.EvPlanProgressed, Payload: ports.PayloadPlanProgressed{TaskID: pl.TaskID}}), nil
}

func (x *StepCritic) maxStepToolFails() int {
	if x.MaxFailsPerTool <= 0 {
		panic("evaluation.StepCritic: MaxFailsPerTool must be > 0")
	}
	return x.MaxFailsPerTool
}

func (x *StepCritic) maxCapabilityFails() int {
	if x.MaxFailsPerCapability < 1 {
		return 2
	}
	return x.MaxFailsPerCapability
}

func (x *StepCritic) bumpCapabilityFailCount(taskState *ports.Task, stepID, capability string) int {
	if taskState.CapabilityFailCount == nil {
		taskState.CapabilityFailCount = make(map[string]int)
	}
	k := rtutils.CapabilityFailCountKey(stepID, capability)
	taskState.CapabilityFailCount[k]++
	return taskState.CapabilityFailCount[k]
}

func (x *StepCritic) trySwitchCapability(ctx context.Context, taskState *ports.Task, exclude string) (string, error) {
	if taskState.RouteSnap == nil {
		return "", fmt.Errorf("step_critic: nil RouteSnap")
	}
	rc := ports.RoutingContext{IntentText: taskState.UserInput.Text}
	frontier, err := x.RouteGraph.Frontier(ctx, rc, taskState.RouteSnap.LastNode, 1)
	if err != nil {
		return "", fmt.Errorf("step_critic: Frontier: %w", err)
	}
	for _, nodeID := range frontier {
		capID, _, ok := routinggraph.ParseNodeID(nodeID)
		if !ok {
			continue
		}
		if capID != exclude {
			return capID, nil
		}
	}
	for _, nodeID := range taskState.RouteSnap.ProcedurePriorNodes {
		capID, _, ok := routinggraph.ParseNodeID(nodeID)
		if !ok {
			continue
		}
		if capID != exclude {
			return capID, nil
		}
	}
	return "", nil
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

func (x *StepCritic) planStepArgsForTool(taskState *ports.Task, stepID string) map[string]any {
	a := argtmpl.RenderPlanStepArguments(taskState, stepID)
	if a == nil {
		return rtutils.PlanStepArguments(taskState, stepID)
	}
	return a
}

func (x *StepCritic) eventUserInputReplan(taskID, text, excludeCap, excludeTool string) *ports.Event {
	return ports.EventPtr(ports.Event{Type: ports.EvUserInput, Payload: ports.PayloadUserInput{
		TaskID:            taskID,
		Text:              text,
		AutoReplan:        true,
		ExcludeCapability: excludeCap,
		ExcludeTool:       excludeTool,
	}})
}

func (x *StepCritic) dropCurrentAndLaterPlanSteps(taskState *ports.Task, failedStepID string) error {
	if taskState.RouteSnap == nil {
		return fmt.Errorf("step_critic: nil RouteSnap")
	}
	if taskState.Plan == nil || len(taskState.Plan.Steps) == 0 {
		return fmt.Errorf("step_critic: cannot replan without existing plan (failed step %q)", failedStepID)
	}
	cut := -1
	for i := range taskState.Plan.Steps {
		if taskState.Plan.Steps[i].ID == failedStepID {
			cut = i
			break
		}
	}
	if cut < 0 {
		return fmt.Errorf("step_critic: failed step %q not found in plan", failedStepID)
	}
	taskState.Plan.Steps = taskState.Plan.Steps[:cut]
	if len(taskState.Plan.Steps) == 0 {
		taskState.Plan = nil
		taskState.RouteSnap.LastNode = ""
		taskState.RouteSnap.LastOut = 0
		return nil
	}
	last := taskState.Plan.Steps[len(taskState.Plan.Steps)-1]
	taskState.RouteSnap.LastNode = routinggraph.NodeID(last.Capability, last.Tool)
	taskState.RouteSnap.LastOut = 0
	return nil
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
			if err := x.dropCurrentAndLaterPlanSteps(taskState, pl.StepID); err != nil {
				return nil, err
			}
			if err := x.Tasks.Put(taskState); err != nil {
				return nil, err
			}
			return x.eventUserInputReplan(pl.TaskID, taskState.UserInput.Text, "", pl.Tool), nil
		}
	}
	if x.shouldRetryTool(taskState, pl) {
		taskState.LastStepDecision = &ports.Decision{Action: ports.ActionRetry, Reason: "tool failed, retry"}
		if err := x.Tasks.Put(taskState); err != nil {
			return nil, err
		}
		return ports.EventPtr(ports.Event{Type: ports.EvToolCall, Payload: ports.PayloadToolCall{
			TaskID: pl.TaskID, StepID: pl.StepID, Capability: pl.Capability, Tool: pl.Tool, Arguments: x.planStepArgsForTool(taskState, pl.StepID),
		}}), nil
	}
	alt, err := x.trySwitchCapability(ctx, taskState, pl.Capability)
	if err != nil {
		return nil, err
	}
	if alt != "" {
		taskState.LastStepDecision = &ports.Decision{Action: ports.ActionSwitchCapability, Reason: "single tool fail, try other capability"}
		if err := x.dropCurrentAndLaterPlanSteps(taskState, pl.StepID); err != nil {
			return nil, err
		}
		if err := x.Tasks.Put(taskState); err != nil {
			return nil, err
		}
		return x.eventUserInputReplan(pl.TaskID, taskState.UserInput.Text, "", pl.Tool), nil
	}
	taskState.LastStepDecision = &ports.Decision{Action: ports.ActionAccept, Reason: "single tool fail, mark done"}
	return nil, nil
}
