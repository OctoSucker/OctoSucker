package planning

import (
	"context"
	"strings"

	"github.com/OctoSucker/agent/pkg/ports"
)

func (p *Planner) HandleUserInput(ctx context.Context, evt ports.Event) (*ports.Event, error) {
	pl := evt.Payload.(ports.PayloadUserInput)
	sess, err := p.Tasks.GetOrCreate(pl.TaskID)
	if err != nil {
		return nil, err
	}
	if pl.TelegramChatID != 0 {
		sess.UserInput.TelegramChatID = pl.TelegramChatID
		sess.UserInput.IngressChannel = ports.IngressTelegram
	} else {
		sess.UserInput.IngressChannel = ports.IngressHTTP
	}
	if !pl.AutoReplan {
		sess.ReplanAllowed = true
		sess.ReplanCount = 0
	}
	sess.UserInput.Text = pl.Text
	sess.Trace = nil
	sess.ToolFailCount = nil
	sess.CapabilityFailCount = nil
	sess.SkillPriorCaps = nil
	sess.SkillPreferredPath = nil
	sess.ActiveSkillName = ""
	sess.ActiveSkillVariantID = ""
	sess.RoutePolicy = nil
	sess.TransitionPath = nil
	sess.GraphPathMode = p.DefaultGraphPathMode

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
	sess.RoutePolicy = routeDec
	if err := p.Tasks.Put(sess); err != nil {
		return nil, err
	}
	if routeDec.Type == ports.RouteTypeEmbeddingSkill || routeDec.Type == ports.RouteTypeKeywordSkill {
		return ports.EventPtr(ports.Event{Type: ports.EvSkillPlanRequested, Payload: ports.PayloadSkillPlanRequested{TaskID: pl.TaskID}}), nil
	}
	return ports.EventPtr(ports.Event{Type: ports.EvLLMPlanRequested, Payload: ports.PayloadLLMPlanRequested{TaskID: pl.TaskID}}), nil
}
