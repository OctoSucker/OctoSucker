package judge

import (
	"context"
	"fmt"
	"strings"

	"github.com/OctoSucker/agent/internal/runtime/store/task"
	"github.com/OctoSucker/agent/pkg/llmclient"
	"github.com/OctoSucker/agent/pkg/ports"
	rtutils "github.com/OctoSucker/agent/utils"
)

const replanScoreThreshold = 0.55
const maxReplanRounds = 2
const trajSystem = `You are a trajectory critic. Given the plan steps and execution trace, reply in 2-4 sentences: overall quality, any concern, whether safe to show the user. No JSON.`

type TrajectoryCritic struct {
	Tasks         *task.TaskStore
	TrajectoryLLM *llmclient.OpenAI
}

func NewTrajectoryCritic(tasks *task.TaskStore, trajectoryLLM *llmclient.OpenAI) *TrajectoryCritic {
	return &TrajectoryCritic{Tasks: tasks, TrajectoryLLM: trajectoryLLM}
}

func (c *TrajectoryCritic) HandleTrajectoryCheck(ctx context.Context, evt ports.Event) (*ports.Event, error) {
	pl := evt.Payload.(ports.PayloadTrajectoryCheck)
	taskState, ok := c.Tasks.Get(pl.TaskID)
	if !ok || taskState.Plan == nil {
		return nil, nil
	}
	plan := *taskState.Plan
	var score float64
	var baseMsg string
	if len(plan.Steps) == 0 {
		score = 0.0
		baseMsg = "空计划"
	} else if len(taskState.Trace) == 0 {
		score = 0.0
		baseMsg = "无执行轨迹"
	} else {
		okCount := 0
		for _, tr := range taskState.Trace {
			if tr.OK {
				okCount++
			}
		}
		ratio := float64(okCount) / float64(len(taskState.Trace))
		stepRatio := float64(len(taskState.Trace)) / float64(len(plan.Steps))
		if stepRatio > 1 {
			stepRatio = 1
		}
		score = ratio*0.7 + stepRatio*0.3
		if okCount == len(taskState.Trace) && len(taskState.Trace) >= len(plan.Steps) {
			baseMsg = fmt.Sprintf("轨迹完整：%d 步均成功，与计划 %d 步一致。", len(taskState.Trace), len(plan.Steps))
		} else if okCount < len(taskState.Trace) {
			baseMsg = fmt.Sprintf("轨迹风险：%d/%d 步成功，建议复查失败步骤后再交付用户。", okCount, len(taskState.Trace))
		} else {
			baseMsg = fmt.Sprintf("轨迹部分：已执行 %d 步，计划共 %d 步。", len(taskState.Trace), len(plan.Steps))
		}
	}
	var summary string
	if c.TrajectoryLLM == nil {
		summary = baseMsg
	} else {
		var prompt strings.Builder
		for _, st := range plan.Steps {
			prompt.WriteString(st.ID)
			prompt.WriteString(" ")
			prompt.WriteString(st.Goal)
			prompt.WriteString(" [")
			prompt.WriteString(st.Capability)
			prompt.WriteString("]\n")
		}
		prompt.WriteString("--- trace ---\n")
		for _, tr := range taskState.Trace {
			fmt.Fprintf(&prompt, "%s %s ok=%v %s\n", tr.StepID, tr.Tool, tr.OK, tr.Summary)
		}
		ext, err := c.TrajectoryLLM.Complete(ctx, trajSystem, prompt.String())
		if err != nil {
			summary = err.Error()
			score = 0
		} else if ext == "" {
			summary = "engine: empty trajectory llm reply"
			score = 0
		} else {
			summary = baseMsg + "\n---\n" + ext
		}
	}
	var outBuilder strings.Builder
	for _, tr := range taskState.Trace {
		if tr.Summary != "" {
			outBuilder.WriteString(tr.Summary)
			outBuilder.WriteString("\n")
		}
	}
	out := outBuilder.String()
	if out == "" {
		out = taskState.UserInput.Text
	}
	if summary != "" {
		if out != "" {
			out += "\n\n"
		}
		out += summary
	}
	taskState.Reply = out
	taskState.TrajectoryScore = score
	taskState.TrajectorySummary = summary
	if err := c.Tasks.Put(taskState); err != nil {
		return nil, err
	}
	if taskState.ReplanAllowed && taskState.ReplanCount < maxReplanRounds && score < replanScoreThreshold && rtutils.TraceHasFailure(taskState.Trace) {
		taskState.ReplanCount++
		taskState.UserInput.Text = taskState.UserInput.Text + fmt.Sprintf("\n\n[系统：上轮轨迹评分 %.2f，存在失败步骤。请生成更短、更保守的计划。]", float64(score))
		if err := c.Tasks.Put(taskState); err != nil {
			return nil, err
		}
		return ports.EventPtr(ports.Event{Type: ports.EvUserInput, Payload: ports.PayloadUserInput{TaskID: pl.TaskID, Text: taskState.UserInput.Text, AutoReplan: true}}), nil
	}
	return ports.EventPtr(ports.Event{Type: ports.EvTurnFinalized, Payload: ports.PayloadTurnFinalized{TaskID: pl.TaskID}}), nil
}
