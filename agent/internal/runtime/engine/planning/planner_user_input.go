package planning

import (
	"context"
	"strings"

	"github.com/OctoSucker/agent/pkg/ports"
)

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
	taskState.SkillPriorCaps = nil
	taskState.SkillPreferredPath = nil
	taskState.ActiveSkillName = ""
	taskState.ActiveSkillVariantID = ""
	taskState.RoutePolicy = nil
	taskState.TransitionPath = nil
	taskState.GraphPathMode = p.DefaultGraphPathMode

	embeddingHit, err := p.Skills.MatchBestByText(ctx, pl.Text)
	if err != nil {
		return nil, err
	}
	keywordHit := p.Skills.KeywordPlanHit(pl.Text)
	routeDec := &ports.RoutePolicyDecision{Type: ports.RouteTypePlanner, Confidence: 0}
	if embeddingHit != nil && embeddingHit.MatchScore >= p.SkillRouteThreshold {
		routeDec = &ports.RoutePolicyDecision{Type: ports.RouteTypeEmbeddingSkill, Confidence: embeddingHit.MatchScore}
	}
	if keywordHit != nil && keywordHit.SelectedPlan() != nil && p.KeywordConfidence > routeDec.Confidence {
		routeDec = &ports.RoutePolicyDecision{Type: ports.RouteTypeKeywordSkill, Confidence: p.KeywordConfidence}
	}
	graphConfidence := 0.0
	if p.RouteGraph != nil {
		graphConfidence = p.RouteGraph.Confidence(ctx, ports.RoutingContext{IntentText: pl.Text}, "")
	}
	if graphConfidence >= p.GraphRouteThreshold && graphConfidence > routeDec.Confidence {
		routeDec = &ports.RoutePolicyDecision{Type: ports.RouteTypeGraphConfidence, Confidence: graphConfidence}
	}
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
	if routeDec.Type == ports.RouteTypeEmbeddingSkill || routeDec.Type == ports.RouteTypeKeywordSkill {
		return ports.EventPtr(ports.Event{Type: ports.EvSkillPlanRequested, Payload: ports.PayloadSkillPlanRequested{TaskID: pl.TaskID}}), nil
	}
	return ports.EventPtr(ports.Event{Type: ports.EvLLMPlanRequested, Payload: ports.PayloadLLMPlanRequested{TaskID: pl.TaskID}}), nil
}
