package planning

import (
	"context"
	"log"

	types "github.com/OctoSucker/octosucker/internal/runtime/model"
)

func (p *Planner) HandleUserInput(ctx context.Context, pl types.TurnRequest) (*types.Event, error) {
	task, err := p.Tasks.GetOrCreate(pl.TaskID)
	if err != nil {
		return nil, err
	}
	if err := task.SetUserInput(pl.Text); err != nil {
		return nil, err
	}
	if err := task.EnsurePhase(); err != nil {
		return nil, err
	}

	step, route, confidence, err := p.planNextStep(ctx, pl.TaskID, task)
	log.Printf("planner: task=%s route=%s confidence=%.3f", pl.TaskID, route, confidence)
	if err != nil {
		task.AppendTrace("planner failed route=%s confidence=%.2f err=%v", route, confidence, err)
		_ = p.Tasks.Put(task)
		log.Printf("planner: task=%s route=%s err=%v", pl.TaskID, route, err)
		return nil, err
	}
	if err := task.AppendStep(step); err != nil {
		return nil, err
	}
	if err := task.SetPhase(phaseForPlannedTool(step.Node.Tool)); err != nil {
		return nil, err
	}
	task.AppendTrace("planner selected tool=%s route=%s confidence=%.2f goal=%s", step.Node.Tool, route, confidence, step.Goal)
	log.Printf("planner: task=%s phase=%s planned_tool=%s", pl.TaskID, task.Phase, step.Node.Tool)

	if err := p.Tasks.Put(task); err != nil {
		return nil, err
	}
	return types.EventPtr(types.Event{Type: types.EvStepScheduled, Payload: types.StepScheduled{TaskID: pl.TaskID}}), nil
}
