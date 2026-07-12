package execution

import (
	"context"

	"github.com/OctoSucker/octosucker/internal/toolcontract"
)

type ToolRuntime interface {
	Invoke(ctx context.Context, tool string, arguments map[string]any) (toolcontract.Result, error)
	Assess(tool string, arguments map[string]any) toolcontract.Policy
}
