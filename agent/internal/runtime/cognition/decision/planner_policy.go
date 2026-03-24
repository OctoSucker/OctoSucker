package decision

import (
	"context"

	"github.com/OctoSucker/agent/pkg/ports"
)

// PlannerPolicy always yields LLM planner routing (typically last resort in the chain).
type PlannerPolicy struct{}

func (PlannerPolicy) Decide(_ context.Context, _ PolicyState) (PolicyDecision, bool) {
	return PolicyDecision{Mode: ports.RoutePlanner, Confidence: 0, Reason: "planner"}, true
}
