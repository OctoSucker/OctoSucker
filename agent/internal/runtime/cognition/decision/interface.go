package decision

import (
	"context"

	skill "github.com/OctoSucker/agent/internal/runtime/store/skill"
	"github.com/OctoSucker/agent/pkg/ports"
)

// PolicyState is the input snapshot for route policies (skills, graph, user text).
type PolicyState struct {
	UserText        string
	GraphConfidence float64
	EmbeddingHit    *skill.SkillEntry
	KeywordHit      *skill.SkillEntry
}

// PolicyDecision is one routing outcome from a single policy implementation.
type PolicyDecision struct {
	Mode       ports.RouteMode
	Confidence float64
	Reason     string
}

// Policy is one pluggable routing rule. Returns (decision, true) when this policy claims the route.
type Policy interface {
	Decide(context.Context, PolicyState) (PolicyDecision, bool)
}
