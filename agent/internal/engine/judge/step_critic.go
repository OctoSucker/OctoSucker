package judge

import (
	"context"
	"fmt"
	"log"

	"github.com/OctoSucker/agent/pkg/ports"
	routinggraph "github.com/OctoSucker/agent/repo/routing_graph"
	taskstore "github.com/OctoSucker/agent/repo/task"
)

type StepCritic struct {
	Tasks      *taskstore.TaskStore
	RouteGraph *routinggraph.RoutingGraph
}

func NewStepCritic(tasks *taskstore.TaskStore, routeGraph *routinggraph.RoutingGraph) *StepCritic {
	return &StepCritic{Tasks: tasks, RouteGraph: routeGraph}
}

func (x *StepCritic) HandleObservationReady(ctx context.Context, pl ports.PayloadObservation) (*ports.Event, error) {
	task, ok := x.Tasks.Get(pl.TaskID)
	if !ok {
		return nil, fmt.Errorf("step_critic: task %q not found", pl.TaskID)
	}
	step := task.Plan.FindStep(pl.StepID)
	currentNode := step.Node
	prevStep := task.Plan.FindPrevStep(pl.StepID)
	prevNode := prevStep.Node
	if pl.Result.Err != nil {
		if err := x.RouteGraph.RecordTransition(
			ctx,
			task.UserInput,
			prevNode,
			currentNode,
			false,
		); err != nil {
			return nil, fmt.Errorf("step_critic: RecordTransition: %w", err)
		}
		task.ReplanCount++
		if err := task.TruncatePlanFromStep(pl.StepID); err != nil {
			log.Printf("------step_critic:  error: %+v", err)
			return nil, err
		}
		if err := x.Tasks.Put(task); err != nil {
			log.Printf("------step_critic:  error: %+v", err)
			return nil, err
		}
		return ports.EventPtr(ports.Event{
			Type: ports.EvUserInput,
			Payload: ports.PayloadUserInput{
				TaskID: pl.TaskID,
				Text:   task.UserInput,
			}},
		), nil
	} else {
		step.ToolResult = pl.Result
		if err := x.RouteGraph.RecordTransition(
			ctx,
			task.UserInput,
			prevNode,
			currentNode,
			true,
		); err != nil {
			return nil, fmt.Errorf("step_critic: RecordTransition: %w", err)
		}
		task.Plan.MarkDone(pl.StepID)
		if err := x.Tasks.Put(task); err != nil {
			return nil, err
		}
		return ports.EventPtr(ports.Event{
			Type: ports.EvPlanProgressed,
			Payload: ports.PayloadPlanProgressed{
				TaskID: pl.TaskID,
			},
		}), nil
	}
}
