package planning

import (
	"testing"

	"github.com/OctoSucker/octosucker/internal/runtime/model"
	"github.com/OctoSucker/octosucker/internal/toolcontract"
)

var testDescriptor = toolcontract.ToolDescriptor{
	Name: "search",
	InputSchema: map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []any{"query"},
		"properties": map[string]any{
			"query": map[string]any{"type": "string"},
		},
	},
}

func validActJSON() decisionJSON {
	raw := decisionJSON{
		Kind: "act", Goal: "搜索资料", SuccessCriteria: "工具返回与 agent 相关的搜索结果",
		CurrentStepID: "search-sources", Tool: "search", Arguments: map[string]any{"query": "agent"},
		Plan: []planStepJSON{{ID: "search-sources", Goal: "搜索资料", SuccessCriteria: "工具返回与 agent 相关的搜索结果"}},
	}
	raw.Step.Title = "搜索相关资料"
	return raw
}

func TestValidateDecisionRequiresCurrentActionToMatchPlan(t *testing.T) {
	planner := &Planner{}
	turn := model.NewTurn("turn", "conversation", "检查资料", nil)
	raw := validActJSON()
	raw.Plan[0].Goal = "不一致的目标"

	if _, err := planner.validateDecision(raw, turn, []toolcontract.ToolDescriptor{testDescriptor}); err == nil {
		t.Fatal("expected current action and plan mismatch")
	}
}

func TestValidateDecisionRejectsInvalidToolArguments(t *testing.T) {
	planner := &Planner{}
	turn := model.NewTurn("turn", "conversation", "检查资料", nil)
	raw := validActJSON()
	raw.Arguments = map[string]any{"unknown": true}

	if _, err := planner.validateDecision(raw, turn, []toolcontract.ToolDescriptor{testDescriptor}); err == nil {
		t.Fatal("expected schema validation error")
	}
}

func TestValidateDecisionRequiresSuccessCriteria(t *testing.T) {
	planner := &Planner{}
	turn := model.NewTurn("turn", "conversation", "检查资料", nil)
	raw := validActJSON()
	raw.SuccessCriteria = ""

	if _, err := planner.validateDecision(raw, turn, []toolcontract.ToolDescriptor{testDescriptor}); err == nil {
		t.Fatal("expected missing success criteria error")
	}
}

func TestValidateDecisionRejectsExactFailedAction(t *testing.T) {
	planner := &Planner{}
	turn := model.NewTurn("turn", "conversation", "检查资料", nil)
	raw := validActJSON()
	action := model.Action{Tool: raw.Tool, Arguments: raw.Arguments}
	step := turn.BeginStep(action)
	step.Assessment = model.Assessment{RoutingOutcome: model.RoutingWrongRoute}
	turn.CompleteStep(step, nil)

	if _, err := planner.validateDecision(raw, turn, []toolcontract.ToolDescriptor{testDescriptor}); err == nil {
		t.Fatal("expected repeated failed action error")
	}
}

func TestValidateDecisionRequiresChineseStepTitleForChineseGoal(t *testing.T) {
	planner := &Planner{}
	turn := model.NewTurn("turn", "conversation", "检查资料", nil)
	raw := validActJSON()
	raw.Step.Title = "Search sources"

	if _, err := planner.validateDecision(raw, turn, []toolcontract.ToolDescriptor{testDescriptor}); err == nil {
		t.Fatal("expected language validation error")
	}
}

func TestValidateRespondDisposition(t *testing.T) {
	planner := &Planner{}
	turn := model.NewTurn("turn", "conversation", "回答问题", nil)
	raw := decisionJSON{Kind: "respond", Disposition: "clarify"}
	raw.Step.Title = "补充必要信息"

	decision, err := planner.validateDecision(raw, turn, nil)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Disposition != model.ResponseClarify {
		t.Fatalf("disposition = %q", decision.Disposition)
	}
}
