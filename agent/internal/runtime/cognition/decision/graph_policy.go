package decision

import (
	"context"

	"github.com/OctoSucker/agent/pkg/ports"
)

// GraphPolicy routes using aggregate routing-graph confidence for the current intent.
type GraphPolicy struct {
	Threshold float64
}

func (p GraphPolicy) Decide(_ context.Context, s PolicyState) (PolicyDecision, bool) {
	if s.GraphConfidence >= p.Threshold {
		return PolicyDecision{Mode: ports.RouteGraph, Confidence: s.GraphConfidence, Reason: "graph_confidence"}, true
	}
	return PolicyDecision{}, false
}
