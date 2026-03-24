package decision

import (
	"context"

	"github.com/OctoSucker/agent/pkg/ports"
)

// SkillPolicy routes to an existing learned / registered skill via embedding or keyword match.
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
