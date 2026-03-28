package planning

import (
	"fmt"

	"github.com/OctoSucker/agent/pkg/ports"
)

const telegramReplyStepTempID = "__telegram_user_reply__"

func (p *Planner) capabilityForTool(toolName string) string {
	if p.CapRegistry == nil || toolName == "" {
		return ""
	}
	for capName, def := range p.CapRegistry.AllCapabilities() {
		for _, t := range def.Tools {
			if t == toolName {
				return capName
			}
		}
	}
	return ""
}

// maybeAppendTelegramUserReplyStep adds a final send_telegram_message step when the user spoke on Telegram
// and the plan would otherwise leave no user-visible chat bubble (data-only tools).
func (p *Planner) maybeAppendTelegramUserReplyStep(taskState *ports.Task, plan *ports.Plan) (*ports.Plan, error) {
	if plan == nil || len(plan.Steps) == 0 || taskState == nil {
		return plan, nil
	}
	if taskState.UserInput.TelegramChatID == 0 {
		return plan, nil
	}
	last := plan.Steps[len(plan.Steps)-1]
	if last.Tool == "send_telegram_message" {
		return plan, nil
	}
	capTel := p.capabilityForTool("send_telegram_message")
	if capTel == "" {
		return plan, nil
	}
	dep := last.ID
	if dep == "" {
		return nil, fmt.Errorf("planner: cannot append telegram reply: last step has empty id")
	}
	steps := make([]ports.PlanStep, len(plan.Steps)+1)
	for i := range plan.Steps {
		steps[i] = plan.Steps[i].Clone()
	}
	steps[len(steps)-1] = ports.PlanStep{
		ID:         telegramReplyStepTempID,
		Goal:       "Send the prior step result to the user on Telegram",
		Capability: capTel,
		Tool:       "send_telegram_message",
		DependsOn:  []string{dep},
		Arguments: map[string]any{
			"chat_id": taskState.UserInput.TelegramChatID,
			"text":    "{{last}}",
		},
		Status: "pending",
	}
	out := &ports.Plan{Steps: steps}
	return reassignPlanStepIDsWithUUID(out)
}
