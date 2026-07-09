package execution

import (
	"context"

	types "github.com/OctoSucker/octosucker/internal/runtime/model"
)

type ToolInvoker interface {
	Invoke(ctx context.Context, tool string, arguments map[string]any) (types.ToolResult, error)
}

type ToolPolicyAssessor interface {
	Assess(tool string, arguments map[string]any) types.ToolPolicy
}
