package planning

import (
	"context"
	"strings"

	"github.com/OctoSucker/agent/pkg/ports"
)

const graphRouteThreshold = 0.7
const procedureRouteThreshold = 0.9
const keywordConfidenceThreshold = 0.92

func (p *Planner) HandleUserInput(ctx context.Context, evt ports.Event) (*ports.Event, error) {
	pl := evt.Payload.(ports.PayloadUserInput)
	taskState, err := p.Tasks.GetOrCreate(pl.TaskID)
	if err != nil {
		return nil, err
	}
	if pl.TelegramChatID != 0 {
		taskState.UserInput.TelegramChatID = pl.TelegramChatID
		taskState.UserInput.IngressChannel = ports.IngressTelegram
	} else {
		taskState.UserInput.IngressChannel = ports.IngressHTTP
	}
	if !pl.AutoReplan {
		taskState.ReplanAllowed = true
		taskState.ReplanCount = 0
	}
	taskState.UserInput.Text = pl.Text
	taskState.Trace = nil
	taskState.ToolFailCount = nil
	taskState.CapabilityFailCount = nil
	taskState.ProcedurePriorCaps = nil
	taskState.ProcedurePreferredPath = nil
	taskState.ActiveProcedureName = ""
	taskState.ActiveProcedureVariantID = ""
	taskState.RoutePolicy = nil
	taskState.TransitionPath = nil
	taskState.GraphPathMode = p.DefaultGraphPathMode

	embeddingHit, err := p.Procedures.MatchBestByText(ctx, pl.Text)
	if err != nil {
		return nil, err
	}
	keywordHit := p.Procedures.KeywordPlanHit(pl.Text)

	// default to planner
	routeDec := &ports.RoutePolicyDecision{Type: ports.RouteTypePlanner, Confidence: 0}

	// embedding procedure
	if embeddingHit != nil && embeddingHit.MatchScore >= procedureRouteThreshold {
		routeDec = &ports.RoutePolicyDecision{Type: ports.RouteTypeEmbeddingProcedure, Confidence: embeddingHit.MatchScore}
	}

	// keyword procedure
	if keywordHit != nil && keywordHit.SelectedPlan() != nil && keywordConfidenceThreshold > routeDec.Confidence {
		routeDec = &ports.RoutePolicyDecision{Type: ports.RouteTypeKeywordProcedure, Confidence: keywordConfidenceThreshold}
	}

	// graph confidence
	graphConfidence := p.RouteGraph.Confidence(ctx, ports.RoutingContext{IntentText: pl.Text}, "")
	if graphConfidence >= graphRouteThreshold && graphConfidence > routeDec.Confidence {
		routeDec = &ports.RoutePolicyDecision{Type: ports.RouteTypeGraphConfidence, Confidence: graphConfidence}
	}

	// heuristic complex request
	lower := strings.ToLower(pl.Text)
	if pl.Text != "" && (strings.Contains(lower, "然后") || strings.Contains(lower, " and ") || strings.Contains(lower, " then ")) {
		if 0.05 > routeDec.Confidence {
			routeDec = &ports.RoutePolicyDecision{Type: ports.RouteTypeHeuristicComplexRequest, Confidence: 0.05}
		}
	}
	taskState.RoutePolicy = routeDec
	if err := p.Tasks.Put(taskState); err != nil {
		return nil, err
	}

	if routeDec.Type == ports.RouteTypeEmbeddingProcedure || routeDec.Type == ports.RouteTypeKeywordProcedure {
		return ports.EventPtr(ports.Event{Type: ports.EvProcedurePlanRequested, Payload: ports.PayloadProcedurePlanRequested{TaskID: pl.TaskID}}), nil
	} else {
		return ports.EventPtr(ports.Event{Type: ports.EvLLMPlanRequested, Payload: ports.PayloadLLMPlanRequested{TaskID: pl.TaskID}}), nil
	}
}
