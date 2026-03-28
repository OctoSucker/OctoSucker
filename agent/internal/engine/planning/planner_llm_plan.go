package planning

import (
	"context"
	"fmt"
	"log"
	"maps"
	"strings"

	"github.com/OctoSucker/agent/pkg/ports"
)

func (p *Planner) buildLLMPlan(ctx context.Context, taskID string, taskState *ports.Task) (*ports.Plan, error) {
	chunks, err := p.RecallCorpus.Recall(ctx, taskState.UserInput.Text, 5)
	if err != nil {
		log.Printf("engine.Dispatcher: recall failed task=%s err=%v", taskID, err)
		return nil, fmt.Errorf("planner: recall: %w", err)
	}

	user := taskState.UserInput.Text
	if len(chunks) > 0 {
		user = "相关记忆：\n" + strings.Join(chunks, "\n---\n") + "\n\n用户请求：\n" + taskState.UserInput.Text
	}
	if taskState.UserInput.TelegramChatID != 0 {
		user += fmt.Sprintf("\n\n[Channel: Telegram; current chat_id is %d. Include it as \"chat_id\" in step arguments when a tool requires chat_id.]", taskState.UserInput.TelegramChatID)
	}
	taskState.RouteSnap.ProcedurePriorNodes = p.Procedures.Match(taskState.UserInput.Text)

	parsed, err := p.completeAndParseLLMPlan(ctx, taskID, user, taskState)
	if err != nil {
		log.Printf("engine.Dispatcher: completeAndParseLLMPlan failed task=%s err=%v", taskID, err)
		return nil, err
	}
	return reassignPlanStepIDsWithUUID(parsed)
}

func (p *Planner) completeAndParseLLMPlan(ctx context.Context, taskID string, user string, taskState *ports.Task) (*ports.Plan, error) {
	system := p.buildPlannerSystemPrompt()
	if taskState != nil && taskState.UserInput.TelegramChatID != 0 {
		system += "\n\nWhen the user message is from Telegram (chat_id appears in the user message), the user only reliably sees text delivered by send_telegram_message. If your plan uses data tools (catalog, exec, skills, etc.) and the user should read the outcome in chat, end the plan with a send_telegram_message step: use the capability that exposes send_telegram_message, set arguments.chat_id to that chat_id, and set arguments.text to a clear human-readable summary (you may reference prior step results in prose). If the plan already ends with send_telegram_message that delivers the answer, do not add another."
	}
	var x llmPlanResponse
	if err := p.PlannerLLM.CompleteJSON(ctx, system, user, &x); err != nil || len(x.Steps) == 0 {
		log.Printf("engine.Dispatcher: invalid plan JSON task=%s err=%v", taskID, err)
		return nil, fmt.Errorf("planner: llm returned invalid or empty plan json")
	}

	validCaps := p.CapRegistry.AllCapabilities()
	parsed := &ports.Plan{}
	for _, st := range x.Steps {
		if st.ID == "" || st.Capability == "" {
			log.Printf("engine.Dispatcher: invalid plan JSON task=%s", taskID)
			return nil, fmt.Errorf("planner: llm returned invalid or empty plan json")
		}
		if _, ok := validCaps[st.Capability]; !ok {
			log.Printf("engine.Dispatcher: invalid plan JSON task=%s", taskID)
			return nil, fmt.Errorf("planner: llm returned invalid or empty plan json")
		}
		parsed.Steps = append(parsed.Steps, ports.PlanStep{
			ID:         st.ID,
			Goal:       st.Goal,
			Capability: st.Capability,
			Tool:       st.Tool,
			DependsOn:  st.DependsOn,
			Arguments:  maps.Clone(st.Arguments),
			Status:     "pending",
		})
	}
	return parsed, nil
}

func (p *Planner) buildPlannerSystemPrompt() string {
	toolApp := ""
	if p.CapRegistry != nil {
		toolApp = p.CapRegistry.PlannerToolAppendix(context.Background())
	}
	system := p.PlanSystemPrompt + "\n\n" + toolApp
	if p.SkillStore != nil {
		if sa := strings.TrimSpace(p.SkillStore.PlannerAppendix()); sa != "" {
			system += "\n\n" + sa
			system += "\n\nWhen a skill matches the user intent, follow that skill's cautions and tool usage guidance while keeping plan steps executable with listed capabilities/tools. Skill-defined tools are also invocable as capability=skills with tool name equal to the MCP name shown in the tools appendix (pattern skillslug__toolslug)."
		}
	}
	var valid map[string]ports.Capability
	if p.CapRegistry != nil {
		valid = p.CapRegistry.AllCapabilities()
	}
	if hint := p.NodeFailures.HintForCapabilities(valid); hint != "" {
		system += "\n\n" + hint
	}
	system += "\n\nEach step may include optional \"tool\" (required when the chosen capability exposes multiple tools): exact tool name from the appendix. Each step may include optional \"arguments\": only keys allowed by that tool's JSON Schema. If one capability runs multiple tools in sequence without a per-step \"tool\", the runtime uses the first tool only—prefer explicit \"tool\" per step when schemas differ."
	system += "\n\nFor exec capability, each step's \"tool\" is the program name (see appendix). Arguments: optional args (string array for flags/paths after argv0), optional work_dir, timeout_sec, env."
	system += "\n\nWhen using exec for workspace files, set work_dir to a path relative to workspace root and keep paths inside args relative to work_dir. Do not use host absolute paths like /Users/... or C:\\... in args."
	return system
}

type llmPlanResponse struct {
	Steps []struct {
		ID         string         `json:"id"`
		Goal       string         `json:"goal"`
		Capability string         `json:"capability"`
		Tool       string         `json:"tool"`
		DependsOn  []string       `json:"depends_on"`
		Arguments  map[string]any `json:"arguments"`
	} `json:"steps"`
}
