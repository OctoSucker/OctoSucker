package decision

import (
	"context"
)

// Router runs multiple Policy implementations and picks the decision with highest Confidence
// (stable tie-break: earlier policy in the slice wins).
type Router struct {
	Policies []Policy
}

func NewSpecificRouter(skillRouteThreshold float64, graphRouteThreshold float64, keywordConfidence float64) *Router {
	return &Router{
		Policies: []Policy{
			SkillPolicy{EmbeddingThreshold: skillRouteThreshold, KeywordConfidence: keywordConfidence},
			GraphPolicy{Threshold: graphRouteThreshold},
			HeuristicPolicy{},
			PlannerPolicy{},
		},
	}
}

// NewRouter builds a router with policies tried in order; higher Confidence wins.
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
