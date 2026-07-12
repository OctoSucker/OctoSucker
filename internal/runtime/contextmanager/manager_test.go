package contextmanager

import (
	"fmt"
	"strings"
	"testing"

	"github.com/OctoSucker/octosucker/internal/runtime/model"
	"github.com/OctoSucker/octosucker/internal/toolcontract"
)

func TestBuildKeepsLatestStepAndCompactsOlderSteps(t *testing.T) {
	turn := model.NewTurn("turn", "conversation", "summarize the final result", nil)
	for i := 0; i < 8; i++ {
		step := turn.AppendStep(model.Action{
			Goal: fmt.Sprintf("goal-%d", i), Tool: fmt.Sprintf("tool-%d", i),
		}, model.Observation{Result: toolcontract.Result{Output: strings.Repeat(fmt.Sprintf("output-%d ", i), 200)}})
		step.Assessment = model.Assessment{
			Progress: model.ProgressContinue, RoutingOutcome: model.RoutingHelpful,
			RoutingReason: model.RoutingReasonNecessaryPrerequisite, Summary: fmt.Sprintf("summary-%d", i),
		}
	}

	manager := New(Limits{PlannerTokens: 2200})
	snapshot := manager.Build(AudiencePlanner, Input{Turn: turn})

	if !strings.Contains(snapshot.Trajectory, "Tool: tool-7") {
		t.Fatalf("latest step missing from trajectory:\n%s", snapshot.Trajectory)
	}
	if snapshot.Stats.OmittedSteps == 0 {
		t.Fatalf("expected older steps to be compacted: %+v", snapshot.Stats)
	}
	if !strings.Contains(snapshot.Trajectory, "older steps compacted") {
		t.Fatalf("missing explicit compaction marker:\n%s", snapshot.Trajectory)
	}
}

func TestBuildPrioritizesRelevantToolsWhenCatalogExceedsBudget(t *testing.T) {
	turn := model.NewTurn("turn", "conversation", "search twitter posts", nil)
	tools := make([]toolcontract.ToolDescriptor, 0, 20)
	for i := 0; i < 20; i++ {
		tools = append(tools, toolcontract.ToolDescriptor{
			Name:        fmt.Sprintf("unrelated_tool_%02d", i),
			Description: strings.Repeat("database maintenance ", 80),
			InputSchema: map[string]any{"type": "object"},
		})
	}
	tools = append(tools, toolcontract.ToolDescriptor{
		Name: "twitter_search", Description: strings.Repeat("search twitter posts ", 80), InputSchema: map[string]any{"type": "object"},
	})

	manager := New(Limits{PlannerTokens: 2600})
	snapshot := manager.Build(AudiencePlanner, Input{Turn: turn, Tools: tools})

	if !strings.Contains(snapshot.Tools, "twitter_search") {
		t.Fatalf("relevant tool was omitted:\n%s", snapshot.Tools)
	}
	if snapshot.Stats.OmittedTools == 0 {
		t.Fatalf("expected catalog filtering: %+v", snapshot.Stats)
	}
}

func TestBuildConversationKeepsNewestMessages(t *testing.T) {
	messages := make([]model.Message, 0, 20)
	for i := 0; i < 20; i++ {
		messages = append(messages, model.Message{Role: "user", Content: strings.Repeat(fmt.Sprintf("message-%02d ", i), 40)})
	}
	turn := model.NewTurn("turn", "conversation", "goal", messages)

	manager := New(Limits{EvaluatorTokens: 1600})
	snapshot := manager.Build(AudienceEvaluator, Input{Turn: turn})

	if !strings.Contains(snapshot.Conversation, "message-19") {
		t.Fatalf("newest message missing:\n%s", snapshot.Conversation)
	}
	if snapshot.Stats.OmittedMessages == 0 {
		t.Fatalf("expected old messages to be omitted: %+v", snapshot.Stats)
	}
}
