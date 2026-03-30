package routinggraph

import (
	"context"
	"sort"

	"github.com/OctoSucker/agent/pkg/ports"
	"github.com/OctoSucker/agent/repo/graph"
)

func (s *RoutingGraph) ResyncToolsAndStaticGraph(ctx context.Context) error {
	if err := s.requireRegistry(); err != nil {
		return err
	}
	if err := s.reg.ResyncToolsFromRunners(ctx); err != nil {
		return err
	}
	m := s.reg.AllCapabilities()
	static := staticAdjacencyFromCapabilities(m)
	s.g.ReplaceStatic(static)
	return nil
}

// staticAdjacencyFromCapabilities builds the static adjacency map: synthetic root graph.Node{}
// lists all capability/tool nodes; each node has an empty successor list (planner supplies paths).
func staticAdjacencyFromCapabilities(m map[string]ports.Capability) map[graph.Node][]graph.Node {
	if m == nil {
		m = map[string]ports.Capability{}
	}
	ids := make([]graph.Node, 0)
	for capID, capDef := range m {
		for _, tool := range capDef.Tools {
			n := graph.MakeNode(capID, tool)
			if !n.IsValid() {
				continue
			}
			ids = append(ids, n)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i].String() < ids[j].String() })
	static := make(map[graph.Node][]graph.Node, len(ids)+1)
	static[graph.Node{}] = append([]graph.Node(nil), ids...)
	for _, id := range ids {
		static[id] = nil
	}
	return static
}
