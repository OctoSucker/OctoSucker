package planning

import (
	"context"
	"fmt"
	"log"
	"strings"

	types "github.com/OctoSucker/octosucker/internal/runtime/model"
	rt "github.com/OctoSucker/octosucker/internal/runtime/toolrouting"
	"github.com/OctoSucker/octosucker/internal/tools/mcp"
	"github.com/google/uuid"
)

func newPendingPlanStep(goal string, node rt.Node, arguments map[string]any) *types.PlanStep {
	return &types.PlanStep{
		ID:        uuid.New().String(),
		Goal:      goal,
		Node:      node,
		Arguments: arguments,
		Status:    types.StepStatusPending,
	}
}

func (p *Planner) buildArgumentsForTool(ctx context.Context, userGoal, stepGoal, toolID, priorRunsContext, evidence string) (map[string]any, error) {
	if args, ok := p.deterministicArgumentsForTool(userGoal, toolID, priorRunsContext); ok {
		return args, nil
	}
	systemPrompt, userPrompt, err := p.buildToolArgumentsPromptPair(userGoal, stepGoal, toolID, priorRunsContext, evidence)
	if err != nil {
		return nil, err
	}
	args := make(map[string]any)
	if err := p.PlannerLLM.CompleteJSON(ctx, systemPrompt, userPrompt, &args); err != nil {
		return nil, fmt.Errorf("planner: tool arguments json: %w", err)
	}
	if err := p.repairDeterministicArguments(userGoal, toolID, args); err != nil {
		return nil, err
	}
	return args, nil
}

func (p *Planner) validateArgumentsForTool(toolID string, args map[string]any) error {
	toolSpec, err := p.ToolRegistry.Tool(toolID)
	if err != nil {
		return fmt.Errorf("planner: tool spec %q: %w", toolID, err)
	}
	if err := mcp.ValidateToolArguments(toolID, args, toolSpec.InputSchema); err != nil {
		log.Printf("planner: validate args tool=%s args=%v schema=%v err=%v", toolID, args, toolSpec.InputSchema, err)
		return fmt.Errorf("planner: validate tool arguments tool=%s schema=%v err=%w", toolID, toolSpec.InputSchema, err)
	}
	return nil
}

func (p *Planner) deterministicArgumentsForTool(userGoal, toolID, priorRunsContext string) (map[string]any, bool) {
	switch toolID {
	case "list_tool_providers", "list_skills", "reload_skills":
		return map[string]any{}, true
	case "list_tools_for_provider":
		provider, ok := providerMentionedInText(userGoal, p.providerIDs())
		if !ok {
			provider, ok = providerForIntent(userGoal, p.providerIDs())
		}
		if !ok {
			return nil, false
		}
		return map[string]any{"provider": provider}, true
	default:
		return nil, false
	}
}

func (p *Planner) repairDeterministicArguments(userGoal, toolID string, args map[string]any) error {
	switch toolID {
	case "list_tools_for_provider":
		provider, _ := args["provider"].(string)
		if providerIDExists(provider, p.providerIDs()) {
			return nil
		}
		if fallback, ok := providerForIntent(userGoal, p.providerIDs()); ok {
			args["provider"] = fallback
			return nil
		}
		return fmt.Errorf("planner: list_tools_for_provider provider %q is not available", provider)
	case "read_skill":
		name, _ := args["name"].(string)
		if fixed, ok := normalizeSkillName(name, p.skillNames()); ok {
			args["name"] = fixed
			return nil
		}
		if fixed, ok := readSkillRequest(userGoal, p.skillNames()); ok {
			args["name"] = fixed
			return nil
		}
		return fmt.Errorf("planner: read_skill skill %q is not available", name)
	default:
		return nil
	}
}

func providerMentionedInText(text string, providerIDs []string) (string, bool) {
	s := strings.ToLower(text)
	for _, id := range providerIDs {
		pid := strings.ToLower(strings.TrimSpace(id))
		if pid != "" && strings.Contains(s, pid) {
			return id, true
		}
	}
	return "", false
}

func providerForIntent(text string, providerIDs []string) (string, bool) {
	s := strings.ToLower(text)
	aliases := []struct {
		provider string
		markers  []string
	}{
		{"thinker", []string{"提取", "实体", "关系", "相关性", "relation", "entity", "extract"}},
		{"exec", []string{"运行", "执行", "命令", "shell", "command", "run "}},
		{"skills", []string{"skill", "技能"}},
		{"cronjob", []string{"定时", "计划任务", "cron", "schedule"}},
		{"telegram", []string{"telegram", "电报"}},
	}
	for _, a := range aliases {
		if !providerIDExists(a.provider, providerIDs) {
			continue
		}
		for _, marker := range a.markers {
			if strings.Contains(s, marker) {
				return a.provider, true
			}
		}
	}
	return "", false
}

func providerIDExists(provider string, providerIDs []string) bool {
	p := strings.ToLower(strings.TrimSpace(provider))
	if p == "" {
		return false
	}
	for _, id := range providerIDs {
		if strings.ToLower(strings.TrimSpace(id)) == p {
			return true
		}
	}
	return false
}

func (p *Planner) planNextStep(ctx context.Context, taskID string, task *types.Task) (*types.PlanStep, planningRoute, float64, error) {
	if step, ok := p.usMarketAnalysisWorkflowStep(task); ok {
		return step, planningRouteDeterministic, 1, nil
	}
	if task.Plan == nil || !task.Plan.HasSteps() {
		if step, ok := p.deterministicIntentStep(task.UserInput); ok {
			return step, planningRouteDeterministic, 1, nil
		}
		if skillName, ok := readSkillRequest(task.UserInput, p.skillNames()); ok {
			return newPendingPlanStep(task.UserInput, rt.Node{Tool: "read_skill"}, map[string]any{
				"name": skillName,
			}), planningRouteDeterministic, 1, nil
		}
		if isReloadSkillsRequest(task.UserInput) {
			return newPendingPlanStep(task.UserInput, rt.Node{Tool: "reload_skills"}, map[string]any{}), planningRouteDeterministic, 1, nil
		}
		if isListSkillsRequest(task.UserInput) {
			return newPendingPlanStep(task.UserInput, rt.Node{Tool: "list_skills"}, map[string]any{}), planningRouteDeterministic, 1, nil
		}
		if provider, ok := listToolsForProviderRequest(task.UserInput, p.providerIDs()); ok {
			return newPendingPlanStep(task.UserInput, rt.Node{Tool: "list_tools_for_provider"}, map[string]any{
				"provider": provider,
			}), planningRouteDeterministic, 1, nil
		}
		if isListToolProvidersRequest(task.UserInput) {
			return newPendingPlanStep(task.UserInput, rt.Node{Tool: "list_tool_providers"}, map[string]any{}), planningRouteDeterministic, 1, nil
		}
		if isDirectReplyRequest(task.UserInput) {
			return newPendingPlanStep(task.UserInput, rt.Node{Tool: "natural_language_reply"}, map[string]any{
				"user_message": task.UserInput,
			}), planningRouteDeterministic, 1, nil
		}
	}

	route, confidence := p.decidePlanningRoute(ctx, task.UserInput, task)
	switch route {
	case planningRouteGraph:
		step, err := p.selectGraphStep(ctx, taskID, task)
		return step, route, confidence, err
	case planningRouteLLM:
		step, err := p.selectLLMStep(ctx, taskID, task)
		return step, route, confidence, err
	default:
		return nil, route, confidence, fmt.Errorf("planner: unknown route %q", route)
	}
}

func (p *Planner) providerIDs() []string {
	descs := p.ToolRegistry.ProviderDescriptors()
	out := make([]string, 0, len(descs))
	for _, d := range descs {
		if d.ID != "" {
			out = append(out, d.ID)
		}
	}
	return out
}

func (p *Planner) skillNames() []string {
	descs := p.ToolRegistry.SkillDescriptors()
	out := make([]string, 0, len(descs))
	for _, d := range descs {
		if name, ok := d["name"].(string); ok && name != "" {
			out = append(out, name)
		}
	}
	return out
}
