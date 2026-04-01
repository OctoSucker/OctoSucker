package planning

import (
	"context"

	"github.com/OctoSucker/agent/pkg/ports"
	"github.com/OctoSucker/agent/repo/graph"
)

const graphRouteThreshold = 0.9

func (p *Planner) HandleUserInput(ctx context.Context, pl ports.PayloadUserInput) (*ports.Event, error) {
	task, err := p.Tasks.GetOrCreate(pl.TaskID)
	if err != nil {
		return nil, err
	}
	if task.UserInput == "" {
		task.UserInput = pl.Text
	}

	var buildPlan *ports.Plan
	var lastStep *ports.PlanStep
	var lastNode graph.Node
	if len(task.Plan.Steps) > 0 {
		lastStep = task.Plan.Steps[len(task.Plan.Steps)-1]
		lastNode = lastStep.Node
	}
	g := p.RouteGraph.Confidence(ctx, pl.Text, lastNode)
	if g >= graphRouteThreshold {
		buildPlan, err = p.buildGraphPlan(ctx, pl.TaskID, task, pl)
		if err != nil {
			return nil, err
		}
	} else {
		buildPlan, err = p.buildLLMPlan(ctx, pl.TaskID, task, lastStep.PrimaryText())
		if err != nil {
			return nil, err
		}
	}
	task.Plan.Steps = append(task.Plan.Steps, buildPlan.Steps...)
	if err := p.Tasks.Put(task); err != nil {
		return nil, err
	}
	return ports.EventPtr(ports.Event{Type: ports.EvPlanProgressed, Payload: ports.PayloadPlanProgressed{TaskID: pl.TaskID}}), nil

}
