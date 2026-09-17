package responding

import (
	"errors"
	"testing"

	"github.com/OctoSucker/octosucker/internal/runtime/model"
	"github.com/OctoSucker/octosucker/internal/toolcontract"
)

func TestFallbackAnswerUsesLatestSuccessfulNonEmptyObservation(t *testing.T) {
	turn := model.NewTurn("turn", "conversation", "goal", nil)
	turn.AppendStep(model.Action{Tool: "first"}, model.Observation{Result: toolcontract.Result{Output: "useful result"}.WithInferredMeta("first")})
	turn.AppendStep(model.Action{Tool: "empty"}, model.Observation{Result: toolcontract.Result{Output: ""}.WithInferredMeta("empty")})
	turn.AppendStep(model.Action{Tool: "failed"}, model.Observation{Result: toolcontract.Result{Err: errors.New("failed")}.WithInferredMeta("failed")})

	if got := fallbackAnswer(turn, "fallback reason"); got != "useful result" {
		t.Fatalf("fallback answer = %q", got)
	}
}

func TestFallbackAnswerUsesTerminalReasonWithoutObservation(t *testing.T) {
	turn := model.NewTurn("turn", "conversation", "goal", nil)
	if got := fallbackAnswer(turn, "concrete limitation"); got != "concrete limitation" {
		t.Fatalf("fallback answer = %q", got)
	}
}

func TestStripFence(t *testing.T) {
	if got := stripFence("```markdown\nanswer\n```"); got != "answer" {
		t.Fatalf("stripped answer = %q", got)
	}
}
