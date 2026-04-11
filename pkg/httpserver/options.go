package httpserver

import (
	"context"

	"github.com/OctoSucker/octosucker/store"
)

// KnowledgeGraphReader reads KG rows for the /api/graph endpoint.
type KnowledgeGraphReader interface {
	KnowledgeGraphNodesSelectAll() ([]store.KnowledgeGraphNodeRow, error)
	KnowledgeGraphEdgesSelectAll() ([]store.KnowledgeGraphEdgeRow, error)
}

// Options configures the admin HTTP handler.
type Options struct {
	// IndexHTML is the GET / document (embedded admin shell).
	IndexHTML []byte
	// RunChat handles POST /api/chat (one user message → agent reply lines).
	RunChat func(ctx context.Context, message string) ([]string, error)
	// Graph returns the KG reader for the current request, or nil if unavailable
	// (e.g. workspace DB closed). Evaluated per request.
	Graph func() KnowledgeGraphReader
}
