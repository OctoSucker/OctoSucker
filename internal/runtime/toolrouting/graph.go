package toolrouting

import (
	"fmt"
	"sync"

	"github.com/OctoSucker/octosucker/internal/storage"
)

// Graph is the mutable learned routing graph: catalog tool list, per-edge stats,
// recent transitions, and SQLite backing. Tool invocation and catalog strings live on *internal/tools.Registry.
type Graph struct {
	mu                sync.RWMutex
	db                *storage.DB
	edges             map[storage.EdgeKey]*storage.RoutingEdgeRow
	catalogTools      []*Node // registered tools; Frontier always scores next hop among these
	recentTransitions []storage.ContextTransition
}

// New loads routing_edges / routing_transitions into a graph whose static topology matches the given tool IDs
// (typically toolRegistry.AllToolIDs() after resync).
func New(db *storage.DB, toolIDs []string) (*Graph, error) {
	if db == nil {
		return nil, fmt.Errorf("toolrouting: storage DB is nil")
	}
	g := &Graph{
		db:           db,
		edges:        make(map[storage.EdgeKey]*storage.RoutingEdgeRow),
		catalogTools: toolPtrsFromIDs(toolIDs),
	}
	if err := g.loadFromStore(); err != nil {
		return nil, err
	}
	return g, nil
}

func toolPtrsFromIDs(ids []string) []*Node {
	out := make([]*Node, 0, len(ids))
	for _, id := range ids {
		n := Node{Tool: id}
		out = append(out, &n)
	}
	return out
}

func (g *Graph) loadFromStore() error {
	edgeMap, err := g.db.RoutingEdgesSelectAll()
	if err != nil {
		return err
	}
	recent, err := g.db.RoutingTransitionsSelectAll()
	if err != nil {
		return err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	for k, w := range edgeMap {
		g.edges[k] = w
	}
	g.recentTransitions = recent
	return nil
}
