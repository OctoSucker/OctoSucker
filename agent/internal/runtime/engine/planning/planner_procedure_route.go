package planning

import (
	"context"
	"fmt"
	"log"

	procedure "github.com/OctoSucker/agent/internal/runtime/store/procedure"
	"github.com/OctoSucker/agent/pkg/ports"
)

func (p *Planner) HandleProcedurePlanRequested(ctx context.Context, evt ports.Event) (*ports.Event, error) {
	pl := evt.Payload.(ports.PayloadProcedurePlanRequested)
	taskState, ok := p.Tasks.Get(pl.TaskID)
	if !ok {
		return nil, fmt.Errorf("planner: task %q not found", pl.TaskID)
	}
	var plan *ports.Plan
	var patch ports.Task
	routeType := taskState.RoutePolicy.Type
	switch routeType {
	case ports.RouteTypeEmbeddingProcedure:
		embHit, err := p.Procedures.MatchBestByText(ctx, taskState.UserInput.Text)
		if err != nil {
			return nil, err
		}
		if embHit == nil {
			return nil, fmt.Errorf("planner: routeType=%q but embedding procedure hit is nil", routeType)
		}
		plan = p.buildProcedurePlan(embHit, taskState)
		patch.ProcedurePriorCaps = embHit.Capabilities
		patch.ProcedurePreferredPath = append([]string(nil), embHit.Path...)
		patch.ActiveProcedureName = embHit.Name
		patch.ActiveProcedureVariantID = embHit.SelectedVariantID
		if plan != nil {
			if err := p.Procedures.MarkUsed(embHit.Name, embHit.SelectedVariantID); err != nil {
				return nil, fmt.Errorf("planner: mark procedure used: %w", err)
			}
		}
	case ports.RouteTypeKeywordProcedure:
		kwHit := p.Procedures.KeywordPlanHit(taskState.UserInput.Text)
		if kwHit == nil {
			return nil, fmt.Errorf("planner: routeType=%q but keyword procedure hit is nil", routeType)
		}
		plan = p.buildProcedurePlan(kwHit, taskState)
		patch.ProcedurePriorCaps = p.Procedures.Match(taskState.UserInput.Text)
		patch.ProcedurePreferredPath = kwHit.PreferredPath()
		patch.ActiveProcedureName = kwHit.Name
		patch.ActiveProcedureVariantID = kwHit.SelectedVariantID
		if plan != nil {
			if err := p.Procedures.MarkUsed(kwHit.Name, kwHit.SelectedVariantID); err != nil {
				return nil, fmt.Errorf("planner: mark procedure used: %w", err)
			}
		}
	default:
		return nil, fmt.Errorf("planner: unexpected route type for procedure plan: %q", routeType)
	}
	taskState.ProcedurePriorCaps = patch.ProcedurePriorCaps
	taskState.ProcedurePreferredPath = patch.ProcedurePreferredPath
	taskState.ActiveProcedureName = patch.ActiveProcedureName
	taskState.ActiveProcedureVariantID = patch.ActiveProcedureVariantID
	return p.finalizePlan(pl.TaskID, taskState, plan)
}

func (p *Planner) buildProcedurePlan(e *procedure.ProcedureEntry, taskState *ports.Task) *ports.Plan {
	if e == nil {
		return nil
	}
	v := e.SelectedVariant()
	if v == nil || v.Plan == nil {
		return nil
	}
	inv := procedure.InvocationContext{UserInput: taskState.UserInput.Text, Trace: taskState.Trace}
	pln, err := procedure.InvokeProcedureVariant(v, inv)
	if err != nil {
		log.Printf("planner: procedure invocation failed procedure=%s variant=%s err=%v", e.Name, v.ID, err)
		return nil
	}
	return pln
}
