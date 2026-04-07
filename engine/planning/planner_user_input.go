package planning

import (
	"context"
	"log"

	"github.com/OctoSucker/octosucker/engine/types"
	rt "github.com/OctoSucker/octosucker/repo/routegraph"
)

const graphRouteThreshold = 0.9

func (p *Planner) HandleUserInput(ctx context.Context, pl types.PayloadUserInput) (*types.Event, error) {
	task, err := p.Tasks.GetOrCreate(pl.TaskID)
	if err != nil {
		return nil, err
	}
	task.UserInput = pl.Text

	var buildPlan *types.Plan
	var lastStep *types.PlanStep
	var lastNode rt.Node
	if len(task.Plan.Steps) > 0 {
		lastStep = task.Plan.Steps[len(task.Plan.Steps)-1]
		lastNode = lastStep.Node
	}
	g := p.RouteGraph.Confidence(ctx, pl.Text, lastNode)
	if g >= graphRouteThreshold {
		log.Println("building graph plan")
		buildPlan, err = p.buildGraphPlan(ctx, pl.TaskID, task, pl)
		if err != nil {
			return nil, err
		}
	} else {
		log.Println("building llm plan")
		buildPlan, err = p.buildLLMPlan(ctx, pl.TaskID, task, lastStep.PrimaryText())
		if err != nil {
			return nil, err
		}
	}
	task.Plan.Steps = append(task.Plan.Steps, buildPlan.Steps...)
	if err := p.Tasks.Put(task); err != nil {
		return nil, err
	}
	return types.EventPtr(types.Event{Type: types.EvPlanProgressed, Payload: types.PayloadPlanProgressed{TaskID: pl.TaskID}}), nil

}
