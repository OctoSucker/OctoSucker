// Package learning adapts semantic turn outcomes to optional routing storage.
// Persistence failures are handled by the loop as non-fatal diagnostics.
package learning

import (
	"github.com/OctoSucker/octosucker/internal/runtime/model"
	"github.com/OctoSucker/octosucker/internal/runtime/toolrouting"
)

type TransitionRecorder interface {
	RecordTransition(intent string, cost, latency float64, from, to toolrouting.Node, success bool) error
}

type RoutingRecorder struct {
	graph TransitionRecorder
}

func NewRoutingRecorder(graph TransitionRecorder) *RoutingRecorder {
	return &RoutingRecorder{graph: graph}
}

func (r *RoutingRecorder) RecordOutcome(goal, previousTool, currentTool string, outcome model.RoutingOutcome) error {
	if r == nil || r.graph == nil || outcome == model.RoutingNoSignal {
		return nil
	}
	return r.graph.RecordTransition(
		goal,
		0,
		0,
		toolrouting.Node{Tool: previousTool},
		toolrouting.Node{Tool: currentTool},
		outcome == model.RoutingHelpful,
	)
}
