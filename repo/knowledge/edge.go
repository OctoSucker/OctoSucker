package knowledge

import "fmt"

// Edge is a directed influence from From to To. Positive is true for positive correlation,
// false for negative correlation.
type Edge struct {
	From     string
	To       string
	Positive bool
}

// HasEdge reports whether a directed edge from -> to exists.
func (g *Graph) HasEdge(fromID, toID string) (bool, error) {
	if g.db == nil {
		return false, fmt.Errorf("knowledgegraph: HasEdge: graph has no db")
	}
	return g.db.KnowledgeGraphEdgeExists(fromID, toID)
}

// AddEdge creates a directed edge from -> to with the given correlation sign.
// It returns an error if fromID or toID is empty, either endpoint is missing,
// or the edge already exists.
func (g *Graph) AddEdge(fromID, toID string, positive bool) error {
	if fromID == "" || toID == "" {
		return fmt.Errorf("knowledgegraph: AddEdge: empty from or to")
	}
	if g.db == nil {
		return fmt.Errorf("knowledgegraph: AddEdge: graph has no db")
	}
	fromOK, err := g.db.KnowledgeGraphNodeExists(fromID)
	if err != nil {
		return fmt.Errorf("knowledgegraph: AddEdge: db: %w", err)
	}
	if !fromOK {
		return fmt.Errorf("knowledgegraph: AddEdge: from node %q does not exist", fromID)
	}
	toOK, err := g.db.KnowledgeGraphNodeExists(toID)
	if err != nil {
		return fmt.Errorf("knowledgegraph: AddEdge: db: %w", err)
	}
	if !toOK {
		return fmt.Errorf("knowledgegraph: AddEdge: to node %q does not exist", toID)
	}
	edgeExists, err := g.db.KnowledgeGraphEdgeExists(fromID, toID)
	if err != nil {
		return fmt.Errorf("knowledgegraph: AddEdge: db: %w", err)
	}
	if edgeExists {
		return fmt.Errorf("knowledgegraph: AddEdge: edge %q -> %q already exists", fromID, toID)
	}
	if err := g.db.KnowledgeGraphEdgeInsert(fromID, toID, positive); err != nil {
		return fmt.Errorf("knowledgegraph: AddEdge: db: %w", err)
	}
	return nil
}

// Edge returns the edge from -> to and whether it exists.
func (g *Graph) Edge(fromID, toID string) (Edge, bool, error) {
	if g.db == nil {
		return Edge{}, false, fmt.Errorf("knowledgegraph: Edge: graph has no db")
	}
	row, ok, err := g.db.KnowledgeGraphEdgeSelect(fromID, toID)
	if err != nil {
		return Edge{}, false, fmt.Errorf("knowledgegraph: Edge: db: %w", err)
	}
	if !ok {
		return Edge{}, false, nil
	}
	return Edge{From: row.FromID, To: row.ToID, Positive: row.Positive}, true, nil
}

// AddEdgeIfAbsent inserts the edge when missing. If the edge already exists with the same
// positive flag, it returns nil. If it exists with a conflicting sign, it returns an error.
func (g *Graph) AddEdgeIfAbsent(fromID, toID string, positive bool) error {
	if fromID == "" || toID == "" {
		return fmt.Errorf("knowledgegraph: AddEdgeIfAbsent: empty from or to")
	}
	if g.db == nil {
		return fmt.Errorf("knowledgegraph: AddEdgeIfAbsent: graph has no db")
	}
	e, ok, err := g.Edge(fromID, toID)
	if err != nil {
		return fmt.Errorf("knowledgegraph: AddEdgeIfAbsent: %w", err)
	}
	if ok {
		if e.Positive == positive {
			return nil
		}
		return fmt.Errorf("knowledgegraph: AddEdgeIfAbsent: edge %q -> %q exists with conflicting positive=%v (want %v)", fromID, toID, e.Positive, positive)
	}
	return g.AddEdge(fromID, toID, positive)
}
