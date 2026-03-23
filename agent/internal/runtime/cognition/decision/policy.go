package decision

import (
	"context"
	"strings"

	"github.com/OctoSucker/agent/internal/runtime/store"
	"github.com/OctoSucker/agent/pkg/ports"
)

type PolicyState struct {
	UserText        string
	GraphConfidence float64
	EmbeddingHit    *store.SkillEntry
	KeywordHit      *store.SkillEntry
}

type PolicyDecision struct {
	Mode       ports.RouteMode
	Confidence float64
	Reason     string
}

type Policy interface {
	Decide(context.Context, PolicyState) (PolicyDecision, bool)
}

type SkillPolicy struct {
	EmbeddingThreshold float64
	KeywordConfidence  float64
}

func (p SkillPolicy) Decide(_ context.Context, s PolicyState) (PolicyDecision, bool) {
	if s.EmbeddingHit != nil && s.EmbeddingHit.MatchScore >= p.EmbeddingThreshold {
		return PolicyDecision{Mode: ports.RouteSkill, Confidence: s.EmbeddingHit.MatchScore, Reason: "embedding_skill"}, true
	}
	if s.KeywordHit != nil && s.KeywordHit.SelectedPlan() != nil {
		return PolicyDecision{Mode: ports.RouteSkill, Confidence: p.KeywordConfidence, Reason: "keyword_skill"}, true
	}
	return PolicyDecision{}, false
}

type GraphPolicy struct {
	Threshold float64
}

func (p GraphPolicy) Decide(_ context.Context, s PolicyState) (PolicyDecision, bool) {
	if s.GraphConfidence >= p.Threshold {
		return PolicyDecision{Mode: ports.RouteGraph, Confidence: s.GraphConfidence, Reason: "graph_confidence"}, true
	}
	return PolicyDecision{}, false
}

type PlannerPolicy struct{}

func (PlannerPolicy) Decide(_ context.Context, _ PolicyState) (PolicyDecision, bool) {
	return PolicyDecision{Mode: ports.RoutePlanner, Confidence: 0, Reason: "planner"}, true
}

type HeuristicPolicy struct{}

func (HeuristicPolicy) Decide(_ context.Context, s PolicyState) (PolicyDecision, bool) {
	if s.UserText == "" {
		return PolicyDecision{}, false
	}
	lower := strings.ToLower(s.UserText)
	if strings.Contains(lower, "然后") || strings.Contains(lower, " and ") || strings.Contains(lower, " then ") {
		return PolicyDecision{Mode: ports.RoutePlanner, Confidence: 0.05, Reason: "heuristic_complex_request"}, true
	}
	return PolicyDecision{}, false
}

type Router struct {
	Policies []Policy
}

func NewRouter(policies ...Policy) *Router {
	return &Router{Policies: policies}
}

func (r *Router) Decide(ctx context.Context, s PolicyState) (PolicyDecision, bool) {
	if r == nil {
		return PolicyDecision{}, false
	}
	best := PolicyDecision{}
	bestIdx := -1
	for i, policy := range r.Policies {
		if policy == nil {
			continue
		}
		d, ok := policy.Decide(ctx, s)
		if !ok {
			continue
		}
		if bestIdx < 0 || d.Confidence > best.Confidence || (d.Confidence == best.Confidence && i < bestIdx) {
			best = d
			bestIdx = i
		}
	}
	if bestIdx < 0 {
		return PolicyDecision{}, false
	}
	return best, true
}
