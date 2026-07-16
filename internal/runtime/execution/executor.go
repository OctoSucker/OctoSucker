// Package execution invokes one already validated action. It does not plan,
// evaluate progress, retry, or decide when a turn ends.
package execution

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/OctoSucker/octosucker/internal/runtime/model"
	"github.com/OctoSucker/octosucker/internal/toolcontract"
)

type Executor struct {
	tools ToolRuntime
}

type ApprovalHandler func(ctx context.Context, action model.Action, policy toolcontract.Policy) error

type approvalHandlerKey struct{}

func WithApprovalHandler(ctx context.Context, handler ApprovalHandler) context.Context {
	return context.WithValue(ctx, approvalHandlerKey{}, handler)
}

func New(tools ToolRuntime) (*Executor, error) {
	if tools == nil {
		return nil, errToolsRequired
	}
	return &Executor{tools: tools}, nil
}

func (e *Executor) Execute(ctx context.Context, action model.Action) model.Observation {
	policy := e.tools.Assess(action.Tool, action.Arguments)
	if policy.Risk == "high" {
		if handler, ok := ctx.Value(approvalHandlerKey{}).(ApprovalHandler); ok && handler != nil {
			if err := handler(ctx, action, policy); err != nil {
				result := toolcontract.Result{Err: fmt.Errorf("tool approval: %w", err)}.WithInferredMeta(action.Tool)
				return model.Observation{Result: result, Policy: policy}
			}
		}
	}
	result, err := e.tools.Invoke(ctx, action.Tool, action.Arguments)
	if err != nil {
		result.Err = err
	}
	result = result.WithInferredMeta(action.Tool)
	if result.Err != nil {
		log.Printf("executor: action=%s tool=%s risk=%s error=%s", action.ID, action.Tool, policy.Risk, clip(result.Err.Error(), 320))
	} else {
		log.Printf("executor: action=%s tool=%s risk=%s result_kind=%s count=%d empty=%v", action.ID, action.Tool, policy.Risk, result.Kind, result.Count, result.Empty)
	}
	return model.Observation{Result: result, Policy: policy}
}

type requiredError string

func (e requiredError) Error() string { return string(e) }

const errToolsRequired requiredError = "executor: tool runtime is required"

func clip(s string, max int) string {
	runes := []rune(strings.TrimSpace(s))
	if len(runes) <= max {
		return string(runes)
	}
	return string(runes[:max]) + "..."
}
