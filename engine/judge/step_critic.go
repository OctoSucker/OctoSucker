package judge

import (
	"context"
	"fmt"
	"log"

	"github.com/OctoSucker/octosucker/engine/types"
	"github.com/OctoSucker/octosucker/repo/routegraph"
	"github.com/OctoSucker/octosucker/repo/taskstore"
)

type StepCritic struct {
	Tasks      *taskstore.TaskStore
	RouteGraph *routegraph.Graph
}

func NewStepCritic(tasks *taskstore.TaskStore, routeGraph *routegraph.Graph) *StepCritic {
	return &StepCritic{Tasks: tasks, RouteGraph: routeGraph}
}

func (x *StepCritic) HandleObservationReady(ctx context.Context, pl types.PayloadObservation) (*types.Event, error) {
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
			task.UserInput,
			0, 0,
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
		return types.EventPtr(types.Event{
			Type: types.EvUserInput,
			Payload: types.PayloadUserInput{
				TaskID: pl.TaskID,
				Text:   task.UserInput,
			}},
		), nil
	} else {
		step.ToolResult = pl.Result
		if err := x.RouteGraph.RecordTransition(
			task.UserInput,
			0, 0,
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
		return types.EventPtr(types.Event{
			Type: types.EvPlanProgressed,
			Payload: types.PayloadPlanProgressed{
				TaskID: pl.TaskID,
			},
		}), nil
	}
}
