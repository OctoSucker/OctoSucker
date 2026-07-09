package toolrouting

import "github.com/OctoSucker/octosucker/internal/storage"

const recentTransitionsCap = 200

// RecordTransition updates learned edge stats and recent intent/outcome history, then persists.
func (g *Graph) RecordTransition(intent string, cost, latency float64, from, to Node, success bool) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	k := storage.EdgeKey{From: from.String(), To: to.String()}
	e := g.edges[k]
	if e == nil {
		e = &storage.RoutingEdgeRow{FromTool: k.From, ToTool: k.To}
		g.edges[k] = e
	}
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
	g.appendRecentTransitionLocked(storage.ContextTransition{
		Intent: intent, From: from.String(), To: to.String(), Outcome: success,
	})

	err := g.db.RoutingEdgeUpsert(*e)
	if err != nil {
		return err
	}
	return g.db.RoutingTransitionAppend(intent, from.String(), to.String(), success)
}

func (g *Graph) appendRecentTransitionLocked(ct storage.ContextTransition) {
	g.recentTransitions = append(g.recentTransitions, ct)
	if len(g.recentTransitions) > recentTransitionsCap {
		g.recentTransitions = g.recentTransitions[len(g.recentTransitions)-recentTransitionsCap:]
	}
}
