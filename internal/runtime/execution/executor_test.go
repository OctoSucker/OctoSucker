package execution

import (
	"context"
	"errors"
	"testing"

	"github.com/OctoSucker/octosucker/internal/runtime/model"
	"github.com/OctoSucker/octosucker/internal/toolcontract"
)

type fakeTools struct {
	policy  toolcontract.Policy
	result  toolcontract.Result
	invoked int
}

func (f *fakeTools) Invoke(context.Context, string, map[string]any) (toolcontract.Result, error) {
	f.invoked++
	return f.result, f.result.Err
}

func (f *fakeTools) Assess(string, map[string]any) toolcontract.Policy { return f.policy }

func TestHighRiskActionFailsClosedWithoutApprovalHandler(t *testing.T) {
	tools := &fakeTools{policy: toolcontract.Policy{Risk: "high"}}
	executor, err := New(tools)
	if err != nil {
		t.Fatal(err)
	}

	observation := executor.Execute(context.Background(), model.Action{Tool: "dangerous"})
	if observation.Result.Err == nil {
		t.Fatal("expected approval error")
	}
	if kind, ok := toolcontract.FailureKindOf(observation.Result.Err); !ok || kind != toolcontract.FailureApprovalRequired {
		t.Fatalf("failure kind = %q, %v; want %q", kind, ok, toolcontract.FailureApprovalRequired)
	}
	if tools.invoked != 0 {
		t.Fatalf("tool invoked %d times; want 0", tools.invoked)
	}
}

func TestHighRiskActionRunsAfterApproval(t *testing.T) {
	tools := &fakeTools{policy: toolcontract.Policy{Risk: "high"}, result: toolcontract.Result{Output: "ok"}}
	executor, err := New(tools)
	if err != nil {
		t.Fatal(err)
	}
	ctx := WithApprovalHandler(context.Background(), func(context.Context, model.Action, toolcontract.Policy) error { return nil })

	observation := executor.Execute(ctx, model.Action{Tool: "dangerous"})
	if observation.Result.Err != nil {
		t.Fatal(observation.Result.Err)
	}
	if tools.invoked != 1 {
		t.Fatalf("tool invoked %d times; want 1", tools.invoked)
	}
}

func TestRejectedApprovalIsTypedAndDoesNotInvoke(t *testing.T) {
	tools := &fakeTools{policy: toolcontract.Policy{Risk: "high"}}
	executor, _ := New(tools)
	ctx := WithApprovalHandler(context.Background(), func(context.Context, model.Action, toolcontract.Policy) error {
		return errors.New("rejected by user")
	})

	observation := executor.Execute(ctx, model.Action{Tool: "dangerous"})
	kind, ok := toolcontract.FailureKindOf(observation.Result.Err)
	if !ok || kind != toolcontract.FailureApprovalRejected {
		t.Fatalf("failure kind = %q, %v; want %q", kind, ok, toolcontract.FailureApprovalRejected)
	}
	if tools.invoked != 0 {
		t.Fatalf("tool invoked %d times; want 0", tools.invoked)
	}
}
