package gateway

import (
	"context"

	"github.com/OctoSucker/octosucker/internal/interaction"
	"github.com/OctoSucker/octosucker/internal/storage"
	"github.com/OctoSucker/octosucker/internal/task"
)

// Agent is the runtime surface ingresses need: one user-input turn → reply lines,
// plus workspace DB for the admin graph API. Implemented by internal/runtime.Runtime.
type Agent interface {
	RunTurn(ctx context.Context, conversationID, text string) ([]string, error)
	PlanInteraction(ctx context.Context, messages []string) (*interaction.Interaction, error)
	SubmitAssistantInput(activeTaskID, text string) (task.InputResult, error)
	SubmitInteraction(taskID, interactionID string, values map[string]any) (task.InputResult, error)
	SubmitApproval(taskID, approvalID, decision string) (task.Snapshot, error)
	Task(taskID string) (task.Snapshot, bool)
	WorkspaceDB() storage.KnowledgeGraphReader
}
