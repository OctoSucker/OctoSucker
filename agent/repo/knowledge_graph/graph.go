package knowledgegraph

import (
	"context"
	"fmt"

	"github.com/OctoSucker/agent/model"
	"github.com/OctoSucker/agent/pkg/llmclient"
)

const (
	// DefaultEmbeddingMinCosine is the minimum cosine similarity to treat a query as matching a stored node embedding.
	DefaultEmbeddingMinCosine = 0.82
	// DefaultEmbeddingAmbiguityGap is the minimum gap between the best and second-best cosine to accept a unique match.
	DefaultEmbeddingAmbiguityGap = 0.03
)

// Graph uses AgentDB for nodes and edges; New wires embedding from llm. No in-memory graph cache.
type Graph struct {
	db              *model.AgentDB
	embed           func(context.Context, string) ([]float32, error)
	minCosine       float64
	embeddingMargin float64
}

// New uses db and llm.Embed for AddNode and CanonicalForContext. The caller owns db; Close does not close it.
func New(db *model.AgentDB, llm *llmclient.OpenAI) (*Graph, error) {
	if db == nil {
		return nil, fmt.Errorf("knowledgegraph: New: db is nil")
	}
	if llm == nil {
		return nil, fmt.Errorf("knowledgegraph: New: llm is nil")
	}
	return &Graph{
		db:              db,
		minCosine:       DefaultEmbeddingMinCosine,
		embeddingMargin: DefaultEmbeddingAmbiguityGap,
		embed:           llm.Embed,
	}, nil
}

// Close is a no-op for shared AgentDB handles; the opener must close the database.
func (g *Graph) Close() error {
	return nil
}
