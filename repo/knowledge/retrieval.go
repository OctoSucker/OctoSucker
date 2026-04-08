package knowledge

import (
	"context"
	"fmt"
	"sort"
)

// AllNodeIDs returns every stored node id in arbitrary order.
func (g *Graph) AllNodeIDs() ([]string, error) {
	if g.db == nil {
		return nil, fmt.Errorf("knowledgegraph: AllNodeIDs: graph has no db")
	}
	rows, err := g.db.KnowledgeGraphNodesSelectAll()
	if err != nil {
		return nil, fmt.Errorf("knowledgegraph: AllNodeIDs: db: %w", err)
	}
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.ID
	}
	return out, nil
}

// AllEdges returns every directed edge in arbitrary order.
func (g *Graph) AllEdges() ([]Edge, error) {
	if g.db == nil {
		return nil, fmt.Errorf("knowledgegraph: AllEdges: graph has no db")
	}
	rows, err := g.db.KnowledgeGraphEdgesSelectAll()
	if err != nil {
		return nil, fmt.Errorf("knowledgegraph: AllEdges: db: %w", err)
	}
	out := make([]Edge, len(rows))
	for i, r := range rows {
		out[i] = Edge{FromID: r.FromID, ToID: r.ToID, Positive: r.Positive}
	}
	return out, nil
}

type nodeScore struct {
	id string
	s  float64
}

// TopSimilarNodes embeds query and returns up to k node ids with cosine >= minCosine against stored embeddings.
func (g *Graph) TopSimilarNodes(ctx context.Context, query string, k int, minCosine float64) ([]string, error) {
	if k <= 0 {
		return nil, fmt.Errorf("knowledgegraph: TopSimilarNodes: k must be positive")
	}
	if query == "" {
		return nil, fmt.Errorf("knowledgegraph: TopSimilarNodes: empty query")
	}
	if g.db == nil {
		return nil, fmt.Errorf("knowledgegraph: TopSimilarNodes: graph has no db")
	}
	snap, err := g.embeddingRowsFromDB()
	if err != nil {
		return nil, err
	}
	if len(snap) == 0 {
		return nil, nil
	}
	q, err := g.llm.Embed(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("knowledgegraph: TopSimilarNodes: embed: %w", err)
	}
	var scored []nodeScore
	for _, row := range snap {
		if len(row.v) != len(q) {
			continue
		}
		s := CosineSimilarity(q, row.v)
		if s < minCosine {
			continue
		}
		scored = append(scored, nodeScore{id: row.id, s: s})
	}
	sort.Slice(scored, func(i, j int) bool { return scored[i].s > scored[j].s })
	if len(scored) > k {
		scored = scored[:k]
	}
	out := make([]string, len(scored))
	for i, x := range scored {
		out[i] = x.id
	}
	return out, nil
}
