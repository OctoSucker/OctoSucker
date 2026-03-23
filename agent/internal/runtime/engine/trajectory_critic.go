package engine

import (
	"context"
	"fmt"
	"strings"

	"github.com/OctoSucker/agent/pkg/ports"
)

const replanScoreThreshold = 0.55
const maxReplanRounds = 2

const trajSystem = `You are a trajectory critic. Given the plan steps and execution trace, reply in 2-4 sentences: overall quality, any concern, whether safe to show the user. No JSON.`

func (d *Dispatcher) trajectoryCriticTrajectoryCheck(ctx context.Context, evt ports.Event) ([]ports.Event, error) {
	pl := evt.Payload.(ports.PayloadTrajectoryCheck)
	sess, ok := d.Sessions.Get(pl.SessionID)
	if !ok || sess.Plan == nil {
		return nil, nil
	}
	plan := *sess.Plan
	var score float64
	var baseMsg string
	if len(plan.Steps) == 0 {
		score = 0.0
		baseMsg = "空计划"
	} else if len(sess.Trace) == 0 {
		score = 0.0
		baseMsg = "无执行轨迹"
	} else {
		okCount := 0
		for _, tr := range sess.Trace {
			if tr.OK {
				okCount++
			}
		}
		ratio := float64(okCount) / float64(len(sess.Trace))
		stepRatio := float64(len(sess.Trace)) / float64(len(plan.Steps))
		if stepRatio > 1 {
			stepRatio = 1
		}
		score = ratio*0.7 + stepRatio*0.3
		if okCount == len(sess.Trace) && len(sess.Trace) >= len(plan.Steps) {
			baseMsg = fmt.Sprintf("轨迹完整：%d 步均成功，与计划 %d 步一致。", len(sess.Trace), len(plan.Steps))
		} else if okCount < len(sess.Trace) {
			baseMsg = fmt.Sprintf("轨迹风险：%d/%d 步成功，建议复查失败步骤后再交付用户。", okCount, len(sess.Trace))
		} else {
			baseMsg = fmt.Sprintf("轨迹部分：已执行 %d 步，计划共 %d 步。", len(sess.Trace), len(plan.Steps))
		}
	}
	var summary string
	if d.TrajectoryLLM == nil {
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
		for _, tr := range sess.Trace {
			fmt.Fprintf(&prompt, "%s %s ok=%v %s\n", tr.StepID, tr.Tool, tr.OK, tr.Summary)
		}
		ext, err := d.TrajectoryLLM.Complete(ctx, trajSystem, prompt.String())
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
	var b strings.Builder
	for _, tr := range sess.Trace {
		if tr.Summary != "" {
			b.WriteString(tr.Summary)
			b.WriteString("\n")
		}
	}
	out := b.String()
	if out == "" {
		out = sess.UserInput
	}
	if summary != "" {
		if out != "" {
			out += "\n\n"
		}
		out += summary
	}
	sess.Reply = out
	sess.TrajectoryScore = score
	sess.TrajectorySummary = summary
	if err := d.Sessions.Put(sess); err != nil {
		return nil, err
	}
	if sess.ReplanAllowed && sess.ReplanCount < maxReplanRounds && score < replanScoreThreshold && anyTraceFail(sess.Trace) {
		sess.ReplanCount++
		sess.UserInput = sess.UserInput + fmt.Sprintf("\n\n[系统：上轮轨迹评分 %.2f，存在失败步骤。请生成更短、更保守的计划。]", float64(score))
		if err := d.Sessions.Put(sess); err != nil {
			return nil, err
		}
		return []ports.Event{{Type: ports.EvUserInput, Payload: ports.PayloadUserInput{SessionID: pl.SessionID, Text: sess.UserInput, AutoReplan: true}}}, nil
	} else {
		return []ports.Event{{Type: ports.EvTurnFinalized, Payload: ports.PayloadTurnFinalized{SessionID: pl.SessionID}}}, nil
	}
}

func anyTraceFail(tr []ports.StepTrace) bool {
	for i := range tr {
		if !tr[i].OK {
			return true
		}
	}
	return false
}
