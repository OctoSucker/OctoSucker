package judge

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/OctoSucker/agent/pkg/ports"
	"github.com/OctoSucker/agent/repo/graph"
	"github.com/OctoSucker/agent/repo/plantemplate"
	routinggraph "github.com/OctoSucker/agent/repo/routing_graph"
	taskstore "github.com/OctoSucker/agent/repo/task"
)

const maxFailsPerTool = 2

func observationErrText(obs ports.Observation) string {
	if obs.Err != nil {
		return obs.Err.Error()
	}
	return strings.TrimSpace(obs.Summary)
}

// plannerReplanHintAfterToolFailure is passed to the planner LLM on the next EvUserInput replan.
func plannerReplanHintAfterToolFailure(step *ports.PlanStep, pl ports.PayloadObservation) string {
	var b strings.Builder
	if step != nil && strings.TrimSpace(step.Goal) != "" {
		fmt.Fprintf(&b, "Failed step goal: %s\n", strings.TrimSpace(step.Goal))
	}
	fmt.Fprintf(&b, "Failed invocation: capability=%s tool=%s\n", pl.Capability, pl.Tool)
	if step != nil && len(step.Arguments) > 0 {
		raw, err := json.Marshal(step.Arguments)
		if err == nil {
			fmt.Fprintf(&b, "Arguments JSON: %s\n", raw)
		}
	}
	fmt.Fprintf(&b, "Error: %s\n", observationErrText(pl.Obs))
	return strings.TrimSpace(b.String())
}

type StepCritic struct {
	Tasks      *taskstore.TaskStore
	RouteGraph *routinggraph.RoutingGraph
}

func NewStepCritic(tasks *taskstore.TaskStore, routeGraph *routinggraph.RoutingGraph) *StepCritic {
	return &StepCritic{Tasks: tasks, RouteGraph: routeGraph}
}

func (x *StepCritic) HandleObservationReady(ctx context.Context, evt ports.Event) (*ports.Event, error) {
	pl := evt.Payload.(ports.PayloadObservation)
	obs := pl.Obs
	task, ok := x.Tasks.Get(pl.TaskID)
	if !ok {
		return nil, fmt.Errorf("step_critic: task %q not found", pl.TaskID)
	}
	step := task.Plan.FindStep(pl.StepID)

	if task.RouteSnap == nil {
		return nil, fmt.Errorf("step_critic: nil RouteSnap")
	}
	currentNode := graph.MakeNode(pl.Capability, pl.Tool)

	if obs.Err != nil {
		if err := x.RouteGraph.RecordTransition(
			ctx,
			task.UserInput.Text,
			task.RouteSnap.LastNode,
			currentNode,
			false,
		); err != nil {
			return nil, fmt.Errorf("step_critic: RecordTransition: %w", err)
		}
		task.ToolFailureTotal++
		if task.ToolFailureTotal > maxTotalToolFailuresPerTurn {
			return x.abortStepCriticTurn(task, pl,
				fmt.Sprintf("本回合工具调用失败次数过多（已记录 %d 次，上限 %d），已停止执行以免空转。最后错误：%s",
					task.ToolFailureTotal, maxTotalToolFailuresPerTurn, observationErrText(obs)))
		}
		step.ToolFailStreak++
		if step.ToolFailStreak < maxFailsPerTool {
			if err := x.Tasks.Put(task); err != nil {
				return nil, err
			}
			return ports.EventPtr(ports.Event{Type: ports.EvToolCall, Payload: ports.PayloadToolCall{
				TaskID:     pl.TaskID,
				StepID:     pl.StepID,
				Capability: pl.Capability,
				Tool:       pl.Tool,
				Arguments:  plantemplate.RenderPlanStepArguments(task, pl.StepID),
			}}), nil
		}
		if task.ReplanCount >= maxReplansPerTurn {
			return x.abortStepCriticTurn(task, pl,
				fmt.Sprintf("本回合因工具失败触发的自动重规划已达上限（%d 次），已停止执行。最后错误：%s",
					maxReplansPerTurn, observationErrText(obs)))
		}
		task.ReplanCount++
		task.PlannerReplanHint = plannerReplanHintAfterToolFailure(step, pl)
		if err := task.TruncatePlanFromStep(pl.StepID); err != nil {
			return nil, err
		}
		if err := x.Tasks.Put(task); err != nil {
			return nil, err
		}
		return x.eventUserInputReplan(pl.TaskID, task.UserInput.Text, "", ""), nil
	} else {
		step.ToolFailStreak = 0
		step.Obs = obs
		// Success: LastNode→current hop increments edge Success (see graph.RecordRoutingTransition). Failures record the same hop with Failure+=1 in handleSingleToolFailure.
		if err := x.RouteGraph.RecordTransition(
			ctx,
			task.UserInput.Text,
			task.RouteSnap.LastNode,
			currentNode,
			true,
		); err != nil {
			return nil, fmt.Errorf("step_critic: RecordTransition: %w", err)
		}
		task.Plan.MarkDone(pl.StepID)
		task.RouteSnap.LastNode = currentNode
		task.RouteSnap.LastOut = true
		if err := x.Tasks.Put(task); err != nil {
			return nil, err
		}
		return ports.EventPtr(ports.Event{Type: ports.EvPlanProgressed, Payload: ports.PayloadPlanProgressed{TaskID: pl.TaskID}}), nil
	}
}

func (x *StepCritic) abortStepCriticTurn(task *ports.Task, pl ports.PayloadObservation, reply string) (*ports.Event, error) {
	if err := task.TruncatePlanFromStep(pl.StepID); err != nil {
		return nil, fmt.Errorf("step_critic: abort truncate: %w", err)
	}
	task.Reply = reply
	task.TrajectorySummary = ""
	if err := x.Tasks.Put(task); err != nil {
		return nil, err
	}
	return nil, nil
}

func (x *StepCritic) eventUserInputReplan(taskID, text, excludeCap, excludeTool string) *ports.Event {
	return ports.EventPtr(ports.Event{Type: ports.EvUserInput, Payload: ports.PayloadUserInput{
		TaskID:              taskID,
		Text:                text,
		PlannerContinuation: true,
		ExcludeCapability:   excludeCap,
		ExcludeTool:         excludeTool,
	}})
}
