package routinggraph

import (
	"context"
	"fmt"

	"github.com/OctoSucker/agent/repo/capability"
	"github.com/OctoSucker/agent/repo/graph"
)

// RoutingGraph combines routing state (topology + learned weights, SQLite via graph.Graph) with the capability registry
// for tool invocation and planner views. The embedded graph.Graph is concurrency-safe; reg is not.
type RoutingGraph struct {
	reg *capability.CapabilityRegistry
	g   *graph.Graph
}

func (s *RoutingGraph) Confidence(ctx context.Context, intent string, last graph.Node) float64 {
	return s.g.Confidence(ctx, intent, last)
}

func (s *RoutingGraph) Frontier(ctx context.Context, intent string, last graph.Node, lastSuccess bool) ([]graph.Node, error) {
	return s.g.Frontier(ctx, intent, last, lastSuccess)
}

func (s *RoutingGraph) FilterCandidatesOnImmediateEdge(last graph.Node, candidates []graph.Node) []graph.Node {
	return s.g.FilterCandidatesOnImmediateEdge(last, candidates)
}

func (s *RoutingGraph) PickBestByImmediateEdge(ctx context.Context, intent string, last graph.Node, candidates []graph.Node) (graph.Node, bool) {
	return s.g.PickBestByImmediateEdge(ctx, intent, last, candidates)
}

func (s *RoutingGraph) RecordTransition(ctx context.Context, intent string, from, to graph.Node, success bool) error {
	return s.g.RecordRoutingTransition(ctx, intent, 0, 0, from, to, success)
}

// IncTotalRunsAndPersist bumps the completed-trajectory counter used for exploration decay (see graph policy).
func (s *RoutingGraph) IncTotalRunsAndPersist() error {
	if err := s.g.IncTotalRunsAndPersist(); err != nil {
		return fmt.Errorf("routinggraph: %w", err)
	}
	return nil
}
