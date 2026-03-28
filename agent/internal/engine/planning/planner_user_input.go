package planning

import (
	"context"
	"log"
	"strings"

	"github.com/OctoSucker/agent/pkg/ports"
)

const graphRouteThreshold = 0.9
const procedureRouteThreshold = 0.9
const keywordConfidenceThreshold = 0.92

func (p *Planner) HandleUserInput(ctx context.Context, evt ports.Event) (*ports.Event, error) {
	pl := evt.Payload.(ports.PayloadUserInput)
	taskState, err := p.Tasks.GetOrCreate(pl.TaskID)
	if err != nil {
		return nil, err
	}
	keepExistingPlan := pl.AutoReplan && taskState.Plan != nil && len(taskState.Plan.Steps) > 0
	resetTaskForUserInput(taskState, pl, !keepExistingPlan)

	embHit, err := p.Procedures.MatchBestByText(ctx, pl.Text)
	if err != nil {
		return nil, err
	}
	kwHit := p.Procedures.KeywordPlanHit(pl.Text)

	var buildPlan *ports.Plan
	taskState.RouteSnap = &ports.RouteSnap{RouteType: ports.RouteTypePlanner, Confidence: 0}
	if embHit != nil && embHit.MatchScore >= procedureRouteThreshold {
		taskState.RouteSnap = &ports.RouteSnap{RouteType: ports.RouteTypeEmbeddingProcedure, Confidence: embHit.MatchScore}
		buildPlan, err = p.buildPlanFromProcedure(taskState, embHit)
		if err != nil {
			return nil, err
		}
	}
	if kwHit != nil && kwHit.SelectedPlan() != nil && keywordConfidenceThreshold > taskState.RouteSnap.Confidence {
		taskState.RouteSnap = &ports.RouteSnap{RouteType: ports.RouteTypeKeywordProcedure, Confidence: keywordConfidenceThreshold}
		buildPlan, err = p.buildPlanFromProcedure(taskState, kwHit)
		if err != nil {
			return nil, err
		}
	}
	g := p.RouteGraph.Confidence(ctx, ports.RoutingContext{IntentText: pl.Text}, "")
	log.Printf("------planner: graph confidence: %f", g)
	if g >= graphRouteThreshold && g > taskState.RouteSnap.Confidence {
		taskState.RouteSnap = &ports.RouteSnap{RouteType: ports.RouteTypeGraphConfidence, Confidence: g}
		buildPlan, err = p.buildGraphPlan(ctx, pl.TaskID, taskState, pl.ExcludeCapability, pl.ExcludeTool)
		if err != nil {
			return nil, err
		}
	}
	if taskState.RouteSnap.Confidence < 0.5 {
		taskState.RouteSnap = &ports.RouteSnap{RouteType: ports.RouteTypeHeuristicComplexRequest, Confidence: 0.05}
		buildPlan, err = p.buildLLMPlan(ctx, pl.TaskID, taskState)
		if err != nil {
			return nil, err
		}
	}

	taskState.RouteSnap.UserInput = pl.Text
	log.Printf("planner: build plan from %s: %+v", taskState.RouteSnap.RouteType, buildPlan)

	return p.finalizePlan(ctx, pl.TaskID, taskState, buildPlan)
}

func resetTaskForUserInput(taskState *ports.Task, pl ports.PayloadUserInput, resetPlan bool) {
	if pl.TelegramChatID != 0 {
		taskState.UserInput.TelegramChatID = pl.TelegramChatID
		taskState.UserInput.IngressChannel = ports.IngressTelegram
	}
	if !pl.AutoReplan {
		taskState.ReplanAllowed = true
		taskState.ReplanCount = 0
	}
	taskState.UserInput.Text = pl.Text
	if resetPlan {
		taskState.Plan = nil
	}
	taskState.Trace = nil
	taskState.ToolFailCount = nil
	taskState.CapabilityFailCount = nil
	taskState.ActiveProcedureName = ""
	taskState.ActiveProcedureVariantID = ""
	if taskState.RouteSnap != nil {
		taskState.RouteSnap.ProcedurePriorNodes = nil
		taskState.RouteSnap.ProcedurePreferredNodes = nil
		taskState.RouteSnap.LastNode = ""
		taskState.RouteSnap.LastOut = 0
		taskState.RouteSnap.UserInput = pl.Text
	}
	taskState.RouteSnap = nil
	taskState.TransitionPath = nil
}

func isComplexRequest(text string) bool {
	if text == "" {
		return false
	}
	lower := strings.ToLower(text)
	return strings.Contains(lower, "然后") || strings.Contains(lower, " and ") || strings.Contains(lower, " then ")
}
