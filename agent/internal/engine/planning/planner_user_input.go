package planning

import (
	"context"
	"fmt"
	"log"

	"github.com/OctoSucker/agent/pkg/ports"
	"github.com/OctoSucker/agent/repo/capability/mcp"
	"github.com/OctoSucker/agent/repo/graph"
)

const graphRouteThreshold = 0.9

func (p *Planner) HandleUserInput(ctx context.Context, evt ports.Event) (*ports.Event, error) {
	pl := evt.Payload.(ports.PayloadUserInput)
	task, err := p.Tasks.GetOrCreate(pl.TaskID)
	if err != nil {
		return nil, err
	}

	cont := pl.PlannerContinuation
	keepExistingPlan := cont && task.Plan != nil && len(task.Plan.Steps) > 0
	resetTaskForUserInput(task, pl, !keepExistingPlan, cont)

	var buildPlan *ports.Plan

	g := p.RouteGraph.Confidence(ctx, pl.Text, graph.Node{})
	log.Printf("------planner: graph confidence: %f", g)
	if g >= graphRouteThreshold {
		task.RouteSnap = &ports.RouteSnap{RouteType: ports.RouteTypeGraphConfidence, Confidence: g}
		buildPlan, err = p.buildGraphPlan(ctx, pl.TaskID, task, pl.ExcludeCapability, pl.ExcludeTool)
		if err != nil {
			return nil, err
		}
	}

	if routeSnapConfidence(task.RouteSnap) < 0.5 {
		task.RouteSnap = &ports.RouteSnap{RouteType: ports.RouteTypeHeuristicComplexRequest, Confidence: 0.05}
		buildPlan, err = p.buildLLMPlan(ctx, pl.TaskID, task)
		if err != nil {
			return nil, err
		}
	}
	if buildPlan == nil || len(buildPlan.Steps) == 0 {
		return nil, fmt.Errorf("planner: empty plan")
	}
	task.RouteSnap.UserInput = pl.Text

	for _, st := range buildPlan.Steps {
		t, err := p.RouteGraph.Tool(st.Capability, st.Tool)
		if err != nil {
			return nil, fmt.Errorf("planner: step id=%v: %w", st.ID, err)
		}
		if err := mcp.ValidateToolArguments(st.Tool, st.Arguments, t.InputSchema); err != nil {
			log.Printf("engine.Dispatcher: plan arguments invalid task=%s err=%v", pl.TaskID, err)
			return nil, fmt.Errorf("planner: plan tool arguments: step id=%q capability=%q tool=%q: %w", st.ID, st.Capability, st.Tool, err)
		}
		if st.Status == "" {
			st.Status = "pending"
		}
	}

	task.Plan.Steps = append(task.Plan.Steps, buildPlan.Steps...)
	task.RouteSnap.LastNode = graph.Node{}
	task.RouteSnap.LastOut = true
	task.PlannerReplanHint = ""
	if err := p.Tasks.Put(task); err != nil {
		return nil, err
	}
	return ports.EventPtr(ports.Event{Type: ports.EvPlanProgressed, Payload: ports.PayloadPlanProgressed{TaskID: pl.TaskID}}), nil

}

func resetTaskForUserInput(task *ports.Task, pl ports.PayloadUserInput, resetPlan bool, preserveReplanBudget bool) {
	if pl.TelegramChatID != 0 {
		task.UserInput.TelegramChatID = pl.TelegramChatID
		task.UserInput.IngressChannel = ports.IngressTelegram
	}
	if !preserveReplanBudget {
		task.ReplanCount = 0
		task.ToolFailureTotal = 0
		task.PlannerReplanHint = ""
	}
	task.UserInput.Text = pl.Text
	if resetPlan {
		task.Plan = &ports.Plan{
			Steps: []*ports.PlanStep{},
		}
	}
	if task.RouteSnap != nil {
		task.RouteSnap.LastNode = graph.Node{}
		task.RouteSnap.LastOut = true
		task.RouteSnap.UserInput = pl.Text
	}
	task.RouteSnap = nil
}

func routeSnapConfidence(rs *ports.RouteSnap) float64 {
	if rs == nil {
		return 0
	}
	return rs.Confidence
}
