package graph

import (
	"context"
	"fmt"
)

// RecordRoutingTransition updates edge stats and recent transitions for one hop, then persists.
func (g *Graph) RecordRoutingTransition(_ context.Context, intent string, cost, latency float64, from, to Node, success bool) error {
	toN := to
	if !toN.IsValid() {
		return fmt.Errorf("graph: invalid transition to %q", toN.String())
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	k := Key{From: from, To: toN}
	e := g.ensureEdgeLocked(k)
	if success {
		e.Success += 1.0
	} else {
		e.Failure += 1.0
	}
	total := e.Success + e.Failure
	if total > 0 {
		if cost > 0 {
			e.Cost = (e.Cost*float64(total-1) + cost) / float64(total)
		}
		if latency > 0 {
			e.Latency = (e.Latency*float64(total-1) + latency) / float64(total)
		}
	}
	g.appendRecentTransitionLocked(ContextTransition{
		Intent: intent, From: from.String(), To: toN.String(), Outcome: success,
	})
	return g.persistRoutingTransitionLocked(k, e, g.recentTransitionsCloneLocked())
}
