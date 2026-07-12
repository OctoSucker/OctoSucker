package adminhttp

import (
	"context"

	"github.com/OctoSucker/octosucker/internal/interaction"
	"github.com/OctoSucker/octosucker/internal/storage"
)

// Options configures the admin HTTP handler.
type Options struct {
	// IndexHTML is the GET / document (admin shell). Empty uses the bundled shell from shell/index.html.
	IndexHTML []byte
	// RunChat handles POST /api/chat (one user message → agent reply lines).
	RunChat func(ctx context.Context, conversationID, message string) ([]string, error)
	// PlanInteraction optionally converts assistant replies into a UI schema.
	PlanInteraction func(ctx context.Context, messages []string) (*interaction.Interaction, error)
	// Graph returns the KG reader for the current request, or nil if unavailable
	// (e.g. workspace DB closed). Evaluated per request.
	Graph func() storage.KnowledgeGraphReader
}
