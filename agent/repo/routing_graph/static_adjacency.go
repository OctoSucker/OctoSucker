package routinggraph

import (
	"context"
	"sort"

	"github.com/OctoSucker/agent/repo/graph"
)

func (s *RoutingGraph) ResyncToolsAndStaticGraph(ctx context.Context) error {
	if err := s.requireRegistry(); err != nil {
		return err
	}
	if err := s.reg.ResyncToolsFromBackends(ctx); err != nil {
		return err
	}
	static := staticAdjacencyFromToolIDs(s.reg.AllToolIDs())
	s.g.ReplaceStatic(static)
	return nil
}

// staticAdjacencyFromToolIDs builds the static adjacency map: synthetic root graph.Node{}
// lists all tool vertices; each node has an empty successor list (planner supplies paths).
func staticAdjacencyFromToolIDs(ids []string) map[graph.Node][]*graph.Node {
	if ids == nil {
		ids = []string{}
	}
	nodes := make([]graph.Node, 0, len(ids))
	for _, id := range ids {
		n, ok := graph.ParseNode(id)
		if !ok || !n.IsValid() {
			continue
		}
		nodes = append(nodes, n)
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].String() < nodes[j].String() })
	static := make(map[graph.Node][]*graph.Node, len(nodes)+1)
	entry := make([]*graph.Node, 0, len(nodes))
	for _, id := range nodes {
		n := id
		entry = append(entry, &n)
	}
	static[graph.Node{}] = entry
	for _, id := range nodes {
		static[id] = nil
	}
	return static
}
