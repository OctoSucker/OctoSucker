package knowledgegraph

import (
	"context"
	"fmt"
)

// Node is an entity concept in the knowledge graph (e.g. "战争", "纳斯达克指数").
type Node struct {
	ID string
}

// HasNode reports whether id is stored as a canonical node key (exact match only).
// For semantic equivalence use CanonicalForContext / HasConceptContext (requires embedding and stored vectors).
func (g *Graph) HasNode(id string) (bool, error) {
	if g.db == nil {
		return false, fmt.Errorf("knowledgegraph: HasNode: graph has no db")
	}
	return g.db.KnowledgeGraphNodeExists(id)
}

// AddNode creates a node with id. When embed is configured by New, id is embedded and stored for CanonicalForContext.
func (g *Graph) AddNode(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("knowledgegraph: AddNode: empty id")
	}
	if g.db == nil {
		return fmt.Errorf("knowledgegraph: AddNode: graph has no db")
	}
	exists, err := g.db.KnowledgeGraphNodeExists(id)
	if err != nil {
		return fmt.Errorf("knowledgegraph: AddNode: db: %w", err)
	}
	if exists {
		return fmt.Errorf("knowledgegraph: AddNode: node %q already exists", id)
	}
	embedFn := g.embed
	var blob []byte
	if embedFn != nil {
		v, err := embedFn(ctx, id)
		if err != nil {
			return fmt.Errorf("knowledgegraph: AddNode: embed: %w", err)
		}
		b, err := EncodeEmbeddingF32(v)
		if err != nil {
			return err
		}
		blob = b
	}
	if err := g.db.KnowledgeGraphNodeInsert(id, blob); err != nil {
		return fmt.Errorf("knowledgegraph: AddNode: db: %w", err)
	}
	return nil
}

// Node returns the node for id and whether it was found.
func (g *Graph) Node(id string) (Node, bool, error) {
	if g.db == nil {
		return Node{}, false, fmt.Errorf("knowledgegraph: Node: graph has no db")
	}
	ok, err := g.db.KnowledgeGraphNodeExists(id)
	if err != nil {
		return Node{}, false, fmt.Errorf("knowledgegraph: Node: db: %w", err)
	}
	if !ok {
		return Node{}, false, nil
	}
	return Node{ID: id}, true, nil
}
