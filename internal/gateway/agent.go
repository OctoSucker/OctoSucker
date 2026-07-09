package gateway

import (
	"context"

	"github.com/OctoSucker/octosucker/internal/storage"
)

// Agent is the runtime surface ingresses need: one user-input turn → reply lines,
// plus workspace DB for the admin graph API. Implemented by internal/runtime.Runtime.
type Agent interface {
	RunTurn(ctx context.Context, text string) ([]string, error)
	WorkspaceDB() storage.KnowledgeGraphReader
}
