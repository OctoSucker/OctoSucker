package decision

import (
	"context"
	"strings"

	"github.com/OctoSucker/agent/pkg/ports"
)

// HeuristicPolicy applies lightweight text heuristics (e.g. multi-step phrasing → planner).
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
