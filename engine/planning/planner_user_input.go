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

	var lastStep *types.PlanStep
	if len(task.Plan.Steps) > 0 {
		lastStep = task.Plan.Steps[len(task.Plan.Steps)-1]
	}
	lastNode := rt.Node{}
	if lastStep != nil {
		lastNode = lastStep.Node
	}
	g := p.RouteGraph.Confidence(ctx, pl.Text, lastNode)
	log.Printf("-----planner: task=%s route=graph confidence=%.3f", pl.TaskID, g)
	if g >= graphRouteThreshold {
		buildStep, err := p.buildGraphPlan(ctx, pl.TaskID, task)
		if err != nil {
			log.Printf("planner: task=%s route=graph err=%v", pl.TaskID, err)
			return nil, err
		}
		task.Plan.Steps = append(task.Plan.Steps, buildStep)
	} else {
		buildStep, err := p.buildLLMPlan(ctx, pl.TaskID, task)
		if err != nil {
			return nil, err
		}
		task.Plan.Steps = append(task.Plan.Steps, buildStep)
	}
	log.Printf("planner: task=%s plan_steps=%v", pl.TaskID, task.Plan.Steps[len(task.Plan.Steps)-1].Node.Tool)

	if err := p.Tasks.Put(task); err != nil {
		return nil, err
	}
	return types.EventPtr(types.Event{Type: types.EvPlanProgressed, Payload: types.PayloadPlanProgressed{TaskID: pl.TaskID}}), nil

}
