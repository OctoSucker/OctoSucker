package adminhttp

import (
	"context"

	"github.com/OctoSucker/octosucker/internal/interaction"
	"github.com/OctoSucker/octosucker/internal/storage"
	"github.com/OctoSucker/octosucker/internal/task"
)

// Options configures the admin HTTP handler.
type Options struct {
	// IndexHTML is the GET / document (admin shell). Empty uses the bundled shell from shell/index.html.
	IndexHTML []byte
	// RunChat handles POST /api/chat (one user message → agent reply lines).
	RunChat func(ctx context.Context, conversationID, message string) ([]string, error)
	// PlanInteraction optionally converts assistant replies into a UI schema.
	PlanInteraction func(ctx context.Context, messages []string) (*interaction.Interaction, error)
	// SubmitAssistantInput creates or continues the active desktop task.
	SubmitAssistantInput func(activeTaskID, content string) (task.InputResult, error)
	// SubmitTaskInteraction resumes a task with structured user input.
	SubmitTaskInteraction func(taskID, interactionID string, values map[string]any) (task.InputResult, error)
	// SubmitTaskApproval resumes a task waiting on a high-risk tool decision.
	SubmitTaskApproval func(taskID, approvalID, decision string) (task.Snapshot, error)
	// GetTask returns the current authoritative task snapshot.
	GetTask func(taskID string) (task.Snapshot, bool)
	// Graph returns the KG reader for the current request, or nil if unavailable
	// (e.g. workspace DB closed). Evaluated per request.
	Graph func() storage.KnowledgeGraphReader
}
