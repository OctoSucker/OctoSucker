package planning

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/OctoSucker/octosucker/internal/runtime/contextmanager"
	"github.com/OctoSucker/octosucker/internal/runtime/model"
	"github.com/OctoSucker/octosucker/internal/toolcontract"
	"github.com/google/uuid"
)

const plannerAttempts = 2

type Planner struct {
	catalog    ToolCatalog
	llm        JSONCompleter
	advisor    ToolAdvisor
	contexts   *contextmanager.Manager
	projectCtx string
}

func New(catalog ToolCatalog, llm JSONCompleter, advisor ToolAdvisor, contexts *contextmanager.Manager, projectCtx string) (*Planner, error) {
	if catalog == nil {
		return nil, fmt.Errorf("planner: tool catalog is required")
	}
	if llm == nil {
		return nil, fmt.Errorf("planner: llm is required")
	}
	if contexts == nil {
		return nil, fmt.Errorf("planner: context manager is required")
	}
	return &Planner{catalog: catalog, llm: llm, advisor: advisor, contexts: contexts, projectCtx: strings.TrimSpace(projectCtx)}, nil
}

type decisionJSON struct {
	Kind        string         `json:"kind"`
	Disposition string         `json:"disposition"`
	Goal        string         `json:"goal"`
	Tool        string         `json:"tool"`
	Arguments   map[string]any `json:"arguments"`
	Reason      string         `json:"reason"`
}

func (p *Planner) Decide(ctx context.Context, turn *model.Turn) (model.Decision, error) {
	if turn == nil {
		return model.Decision{}, fmt.Errorf("planner: turn is nil")
	}
	descriptors, err := p.catalog.ToolDescriptors(ctx)
	if err != nil {
		return model.Decision{}, fmt.Errorf("planner: tool catalog: %w", err)
	}
	recommendations := []string(nil)
	if p.advisor != nil {
		recommendations = p.advisor.Recommend(ctx, turn.Goal, turn.LastTool(), 3)
	}

	feedback := ""
	var lastErr error
	for attempt := 0; attempt < plannerAttempts; attempt++ {
		system, user, stats := p.prompts(turn, descriptors, recommendations, feedback)
		if attempt == 0 {
			turn.AppendTrace("%s", stats.Trace())
		}
		var raw decisionJSON
		if err := p.llm.CompleteJSON(ctx, system, user, &raw); err != nil {
			lastErr = fmt.Errorf("planner: decision json: %w", err)
			feedback = lastErr.Error()
			continue
		}
		decision, err := p.validateDecision(raw, turn, descriptors)
		if err == nil {
			return decision, nil
		}
		lastErr = err
		feedback = err.Error()
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("planner: no valid decision")
	}
	return model.Decision{}, lastErr
}

func (p *Planner) validateDecision(raw decisionJSON, turn *model.Turn, descriptors []toolcontract.ToolDescriptor) (model.Decision, error) {
	kind := model.DecisionKind(strings.ToLower(strings.TrimSpace(raw.Kind)))
	reason := strings.TrimSpace(raw.Reason)
	switch kind {
	case model.DecisionRespond:
		disposition := model.ResponseDisposition(strings.ToLower(strings.TrimSpace(raw.Disposition)))
		switch disposition {
		case model.ResponseAnswer, model.ResponseClarify, model.ResponseBlocked:
		default:
			return model.Decision{}, fmt.Errorf("planner: respond decision requires disposition answer, clarify, or blocked")
		}
		if reason == "" {
			reason = "No further tool action is needed."
		}
		return model.Decision{Kind: model.DecisionRespond, Disposition: disposition, Reason: reason}, nil

	case model.DecisionAct:
		toolID := strings.TrimSpace(raw.Tool)
		if toolID == "" {
			return model.Decision{}, fmt.Errorf("planner: act decision requires tool")
		}
		var descriptor *toolcontract.ToolDescriptor
		for i := range descriptors {
			if descriptors[i].Name == toolID {
				descriptor = &descriptors[i]
				break
			}
		}
		if descriptor == nil {
			return model.Decision{}, fmt.Errorf("planner: unavailable tool %q", toolID)
		}
		args := raw.Arguments
		if args == nil {
			args = map[string]any{}
		}
		if err := toolcontract.ValidateArguments(args, descriptor.InputSchema); err != nil {
			return model.Decision{}, fmt.Errorf("planner: invalid arguments for %q: %w", toolID, err)
		}
		goal := strings.TrimSpace(raw.Goal)
		if goal == "" {
			return model.Decision{}, fmt.Errorf("planner: act decision requires a concrete goal")
		}
		action := model.Action{ID: uuid.NewString(), Goal: goal, Tool: toolID, Arguments: args}
		if turn.HasFailedAction(action) {
			encoded, _ := json.Marshal(args)
			return model.Decision{}, fmt.Errorf("planner: exact failed action cannot be repeated: tool=%s arguments=%s", toolID, encoded)
		}
		return model.Decision{Kind: model.DecisionAct, Action: action, Reason: reason}, nil

	default:
		return model.Decision{}, fmt.Errorf("planner: unknown decision kind %q (want act or respond)", raw.Kind)
	}
}
