package planning

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/OctoSucker/agent/pkg/ports"
)

func (p *Planner) HandleLLMPlanRequested(ctx context.Context, evt ports.Event) (*ports.Event, error) {
	pl := evt.Payload.(ports.PayloadLLMPlanRequested)
	taskState, ok := p.Tasks.Get(pl.TaskID)
	if !ok {
		return nil, fmt.Errorf("planner: task %q not found", pl.TaskID)
	}

	var recallText string
	chunks, err := p.RecallCorpus.Recall(ctx, taskState.UserInput.Text, 5)
	if err != nil {
		log.Printf("engine.Dispatcher: recall failed task=%s err=%v", pl.TaskID, err)
		return nil, fmt.Errorf("planner: recall: %w", err)
	}
	if len(chunks) > 0 {
		recallText = strings.Join(chunks, "\n---\n")
	}
	user := taskState.UserInput.Text
	if recallText != "" {
		user = "相关记忆：\n" + recallText + "\n\n用户请求：\n" + taskState.UserInput.Text
	}
	if taskState.UserInput.TelegramChatID != 0 {
		user += fmt.Sprintf("\n\n[Channel: Telegram; current chat_id is %d. Include it as \"chat_id\" in step arguments when a tool requires chat_id.]", taskState.UserInput.TelegramChatID)
	}
	taskState.SkillPriorCaps = p.Skills.Match(taskState.UserInput.Text)

	if len(p.ValidPlanCapabilities) == 0 {
		log.Printf("engine.Dispatcher: invalid plan JSON task=%s", pl.TaskID)
		return nil, fmt.Errorf("planner: llm returned invalid or empty plan json")
	}
	system := p.buildPlannerSystemPrompt()
	parsed, err := p.completeAndParseLLMPlan(ctx, pl.TaskID, system, user)
	if err != nil {
		return nil, err
	}
	return p.finalizePlan(pl.TaskID, taskState, parsed)
}
