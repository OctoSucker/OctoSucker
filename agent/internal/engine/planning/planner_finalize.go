package planning

import (
	"context"
	"fmt"
	"log"
	"strconv"

	"github.com/OctoSucker/agent/internal/store/capability/mcp"
	"github.com/OctoSucker/agent/pkg/ports"
)

func (p *Planner) finalizePlan(ctx context.Context, taskID string, taskState *ports.Task, plan *ports.Plan) (*ports.Event, error) {
	if plan == nil || len(plan.Steps) == 0 {
		return nil, fmt.Errorf("planner: empty plan")
	}
	if taskState.RouteSnap == nil {
		return nil, fmt.Errorf("planner: nil RouteSnap")
	}
	var err error
	plan, err = p.maybeAppendTelegramUserReplyStep(taskState, plan)
	if err != nil {
		return nil, err
	}
	for i := range plan.Steps {
		st := &plan.Steps[i]
		t, err := p.CapRegistry.Tool(ctx, st.Capability, st.Tool)
		if err != nil {
			return nil, fmt.Errorf("planner: step id=%v: %w", st.ID, err)
		}
		if err := mcp.ValidateToolArguments(st.Tool, st.Arguments, t.InputSchema); err != nil {
			log.Printf("engine.Dispatcher: plan arguments invalid task=%s err=%v", taskID, err)
			return nil, fmt.Errorf("planner: plan tool arguments: step id=%q capability=%q tool=%q: %w", st.ID, st.Capability, st.Tool, err)
		}
	}
	for i := range plan.Steps {
		if plan.Steps[i].Status == "" {
			plan.Steps[i].Status = "pending"
		}
	}
	combined, err := appendPlanSteps(taskState.Plan, plan)
	if err != nil {
		return nil, err
	}
	taskState.Plan = combined
	taskState.RouteSnap.LastNode = ""
	taskState.RouteSnap.LastOut = 0
	if err := p.Tasks.Put(taskState); err != nil {
		return nil, err
	}
	return ports.EventPtr(ports.Event{Type: ports.EvPlanProgressed, Payload: ports.PayloadPlanProgressed{TaskID: taskID}}), nil
}

func appendPlanSteps(base, suffix *ports.Plan) (*ports.Plan, error) {
	if suffix == nil || len(suffix.Steps) == 0 {
		return nil, fmt.Errorf("planner: appendPlanSteps requires non-empty suffix plan")
	}
	if base == nil || len(base.Steps) == 0 {
		out := &ports.Plan{Steps: make([]ports.PlanStep, len(suffix.Steps))}
		for i := range suffix.Steps {
			out.Steps[i] = suffix.Steps[i].Clone()
		}
		return out, nil
	}
	prefix := make([]ports.PlanStep, len(base.Steps))
	prefixIDs := make(map[string]struct{}, len(base.Steps))
	for i := range base.Steps {
		prefix[i] = base.Steps[i].Clone()
		prefixIDs[prefix[i].ID] = struct{}{}
	}
	idMap := make(map[string]string, len(suffix.Steps))
	for i := range suffix.Steps {
		oldID := suffix.Steps[i].ID
		if oldID == "" {
			oldID = strconv.Itoa(i + 1)
		}
		idMap[oldID] = strconv.Itoa(len(prefix) + 1 + i)
	}
	rebuilt := make([]ports.PlanStep, len(suffix.Steps))
	for i := range suffix.Steps {
		st := suffix.Steps[i].Clone()
		oldID := st.ID
		if oldID == "" {
			oldID = strconv.Itoa(i + 1)
		}
		mappedID, ok := idMap[oldID]
		if !ok {
			return nil, fmt.Errorf("planner: appendPlanSteps: missing mapped id for %q", oldID)
		}
		st.ID = mappedID
		st.Status = "pending"
		deps := make([]string, 0, len(st.DependsOn))
		for _, dep := range st.DependsOn {
			if mappedDep, ok := idMap[dep]; ok {
				deps = append(deps, mappedDep)
				continue
			}
			if _, ok := prefixIDs[dep]; ok {
				deps = append(deps, dep)
				continue
			}
			return nil, fmt.Errorf("planner: appendPlanSteps: unresolved dependency %q", dep)
		}
		st.DependsOn = deps
		rebuilt[i] = st
	}
	return &ports.Plan{Steps: append(prefix, rebuilt...)}, nil
}
