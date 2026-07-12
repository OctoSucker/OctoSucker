package planning

import (
	"context"

	"github.com/OctoSucker/octosucker/internal/toolcontract"
)

type JSONCompleter interface {
	CompleteJSON(ctx context.Context, system, user string, out any) error
}

type ToolCatalog interface {
	ToolDescriptors(ctx context.Context) ([]toolcontract.ToolDescriptor, error)
	SkillDescriptors() []map[string]any
}

// ToolAdvisor supplies conservative historical hints. A recommendation never
// bypasses the planner or argument validation.
type ToolAdvisor interface {
	Recommend(ctx context.Context, goal, previousTool string, limit int) []string
}
