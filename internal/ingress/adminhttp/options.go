package adminhttp

import (
	"context"

	"github.com/OctoSucker/octosucker/internal/storage"
)

// Options configures the admin HTTP handler.
type Options struct {
	// IndexHTML is the GET / document (admin shell). Empty uses the bundled shell from shell/index.html.
	IndexHTML []byte
	// RunChat handles POST /api/chat (one user message → agent reply lines).
	RunChat func(ctx context.Context, message string) ([]string, error)
	// Graph returns the KG reader for the current request, or nil if unavailable
	// (e.g. workspace DB closed). Evaluated per request.
	Graph func() storage.KnowledgeGraphReader
}
