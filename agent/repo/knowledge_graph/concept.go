package knowledgegraph

import (
	"context"
	"fmt"
)

// CanonicalFor resolves term only to an exact canonical node id (no embedding API).
func (g *Graph) CanonicalFor(term string) (canonical string, ok bool, err error) {
	if term == "" {
		return "", false, nil
	}
	if g.db == nil {
		return "", false, fmt.Errorf("knowledgegraph: CanonicalFor: graph has no db")
	}
	exists, err := g.db.KnowledgeGraphNodeExists(term)
	if err != nil {
		return "", false, fmt.Errorf("knowledgegraph: CanonicalFor: db: %w", err)
	}
	if exists {
		return term, true, nil
	}
	return "", false, nil
}

// CanonicalForContext resolves term: exact id, then embedding cosine similarity when embed is configured and vectors exist.
func (g *Graph) CanonicalForContext(ctx context.Context, term string) (canonical string, ok bool, err error) {
	if term == "" {
		return "", false, nil
	}
	if g.db == nil {
		return "", false, fmt.Errorf("knowledgegraph: CanonicalForContext: graph has no db")
	}
	exists, err := g.db.KnowledgeGraphNodeExists(term)
	if err != nil {
		return "", false, fmt.Errorf("knowledgegraph: CanonicalForContext: db: %w", err)
	}
	if exists {
		return term, true, nil
	}
	embedFn := g.embed
	minC := g.minCosine
	margin := g.embeddingMargin
	if embedFn == nil {
		return "", false, nil
	}
	snap, err := g.embeddingRowsFromDB()
	if err != nil {
		return "", false, err
	}
	if len(snap) == 0 {
		return "", false, nil
	}
	q, eerr := embedFn(ctx, term)
	if eerr != nil {
		return "", false, eerr
	}
	if id, ok := bestCosineMatch(q, snap, minC, margin); ok {
		return id, true, nil
	}
	return "", false, nil
}

func (g *Graph) embeddingRowsFromDB() ([]embeddingRow, error) {
	rows, err := g.db.KnowledgeGraphNodesSelectAll()
	if err != nil {
		return nil, fmt.Errorf("knowledgegraph: embeddingRowsFromDB: %w", err)
	}
	out := make([]embeddingRow, 0, len(rows))
	for _, row := range rows {
		if len(row.Embedding) == 0 {
			continue
		}
		vec, err := DecodeEmbeddingF32(row.Embedding)
		if err != nil {
			return nil, fmt.Errorf("knowledgegraph: embeddingRowsFromDB: node %q: %w", row.ID, err)
		}
		cp := make([]float32, len(vec))
		copy(cp, vec)
		out = append(out, embeddingRow{id: row.ID, v: cp})
	}
	return out, nil
}

type embeddingRow struct {
	id string
	v  []float32
}

func bestCosineMatch(q []float32, rows []embeddingRow, minCosine, ambiguityMargin float64) (string, bool) {
	var bestID string
	var best, second float64 = -2, -2
	for _, row := range rows {
		if len(row.v) != len(q) {
			continue
		}
		s := CosineSimilarity(q, row.v)
		if s > best {
			second = best
			best = s
			bestID = row.id
		} else if s > second {
			second = s
		}
	}
	if bestID == "" || best < minCosine {
		return "", false
	}
	if second > best-ambiguityMargin {
		return "", false
	}
	return bestID, true
}

// HasConcept reports whether term is an exact canonical node id.
func (g *Graph) HasConcept(term string) (bool, error) {
	_, ok, err := g.CanonicalFor(term)
	return ok, err
}

// HasConceptContext reports whether term resolves via CanonicalForContext.
func (g *Graph) HasConceptContext(ctx context.Context, term string) (bool, error) {
	_, ok, err := g.CanonicalForContext(ctx, term)
	return ok, err
}
