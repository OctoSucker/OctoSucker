// Task state persisted per executor turn.
package model

import (
	"encoding/json"
	"fmt"
	"strings"
)

type Task struct {
	ID                string    `json:"id"` // UUID；SQLite PRIMARY KEY
	UserInput         string    `json:"user_input"`
	Plan              *Plan     `json:"plan,omitempty"`
	Reply             string    `json:"reply"`
	Phase             TaskPhase `json:"phase,omitempty"`
	TrajectorySummary string    `json:"trajectory_summary,omitempty"`
	EvidenceSummary   string    `json:"evidence_summary,omitempty"`
	Trace             []string  `json:"trace,omitempty"`
	ReplanCount       int       `json:"replan_count,omitempty"`
}

type TaskPhase string

const (
	TaskPhaseDiscovery TaskPhase = "discovery"
	TaskPhaseExecution TaskPhase = "execution"
	TaskPhaseSynthesis TaskPhase = "synthesis"
	TaskPhaseDone      TaskPhase = "done"
	TaskPhaseAbort     TaskPhase = "abort"
)

func (t *Task) EnsurePhase() error {
	if t == nil {
		return fmt.Errorf("task: ensure phase: nil task")
	}
	if t.Phase == "" {
		t.Phase = TaskPhaseDiscovery
	}
	return nil
}

func (t *Task) SetPhase(phase TaskPhase) error {
	if t == nil {
		return fmt.Errorf("task: set phase: nil task")
	}
	switch phase {
	case TaskPhaseDiscovery, TaskPhaseExecution, TaskPhaseSynthesis, TaskPhaseDone, TaskPhaseAbort:
		t.Phase = phase
		return nil
	default:
		return fmt.Errorf("task: invalid phase %q", phase)
	}
}

func (t *Task) SetUserInput(text string) error {
	if t == nil {
		return fmt.Errorf("task: set user input: nil task")
	}
	t.UserInput = text
	return nil
}

func (t *Task) AppendStep(step *PlanStep) error {
	if t == nil {
		return fmt.Errorf("task: append step: nil task")
	}
	if step == nil {
		return fmt.Errorf("task: append step: nil step")
	}
	if t.Plan == nil {
		t.Plan = NewPlan()
	}
	t.Plan.Steps = append(t.Plan.Steps, step)
	return nil
}

func (t *Task) MarkStepDone(stepID string, result ToolResult) error {
	if t == nil || t.Plan == nil {
		return fmt.Errorf("task: mark step done: nil task or plan")
	}
	step := t.Plan.FindStep(stepID)
	if step == nil {
		return fmt.Errorf("task: mark step done: step %q not found", stepID)
	}
	step.ToolResult = result.WithInferredMeta(step.Node.Tool)
	t.Plan.MarkDone(stepID)
	t.AppendEvidenceFromStep(step)
	return nil
}

func (t *Task) AppendEvidenceFromStep(step *PlanStep) {
	if t == nil || step == nil {
		return
	}
	line := stepEvidenceLine(step)
	if strings.TrimSpace(line) == "" {
		return
	}
	current := strings.TrimSpace(t.EvidenceSummary)
	if current == "" {
		t.EvidenceSummary = line
		return
	}
	t.EvidenceSummary = truncateEvidenceSummary(current + "\n" + line)
}

func (t *Task) AppendTrace(format string, args ...any) {
	if t == nil {
		return
	}
	line := strings.TrimSpace(fmt.Sprintf(format, args...))
	if line == "" {
		return
	}
	t.Trace = append(t.Trace, truncateRunes(line, 700))
	if len(t.Trace) > 80 {
		t.Trace = t.Trace[len(t.Trace)-80:]
	}
}

func (t *Task) TraceSummary() string {
	if t == nil || len(t.Trace) == 0 {
		return ""
	}
	return strings.Join(t.Trace, "\n")
}

func (t *Task) IncrementReplanCount() error {
	if t == nil {
		return fmt.Errorf("task: increment replan count: nil task")
	}
	t.ReplanCount++
	return nil
}

func (t *Task) MarkCompleted(reply, rationale string) error {
	if t == nil {
		return fmt.Errorf("task: mark completed: nil task")
	}
	t.Reply = reply
	if t.Plan != nil {
		t.TrajectorySummary = fmt.Sprintf("计划 %d 步已全部执行完成。", t.Plan.StepCount()) + "\n---\n" + rationale
	} else {
		t.TrajectorySummary = rationale
	}
	t.Phase = TaskPhaseDone
	t.ReplanCount = 0
	return nil
}

const (
	stepEvidenceMaxRunes = 900
	taskEvidenceMaxRunes = 3600
)

func stepEvidenceLine(step *PlanStep) string {
	if step == nil {
		return ""
	}
	tool := strings.TrimSpace(step.Node.Tool)
	goal := strings.TrimSpace(step.Goal)
	if tool == "" {
		tool = "unknown_tool"
	}
	prefix := "- " + tool
	if goal != "" {
		prefix += " (" + truncateRunes(goal, 80) + ")"
	}
	if step.ToolResult.Err != nil {
		args := ""
		if len(step.Arguments) > 0 {
			if b, err := json.Marshal(step.Arguments); err == nil {
				args = " args=" + truncateRunes(string(b), 220)
			}
		}
		return prefix + args + " failed: " + truncateRunes(strings.TrimSpace(step.ToolResult.Err.Error()), 220)
	}
	meta := fmt.Sprintf("kind=%s count=%d empty=%v", step.ToolResult.Kind, step.ToolResult.Count, step.ToolResult.Empty)
	text := strings.TrimSpace(step.PrimaryText())
	if text == "" {
		return prefix + " returned " + meta
	}
	return prefix + " returned " + meta + ": " + oneLine(truncateRunes(text, stepEvidenceMaxRunes))
}

func truncateEvidenceSummary(s string) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) <= taskEvidenceMaxRunes {
		return s
	}
	return string(r[len(r)-taskEvidenceMaxRunes:]) + "\n[older evidence truncated]"
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(s)), " ")
}

func (t *Task) MarkAborted(reply string) error {
	if t == nil {
		return fmt.Errorf("task: mark aborted: nil task")
	}
	t.Reply = reply
	t.TrajectorySummary = ""
	t.Phase = TaskPhaseAbort
	t.ReplanCount = 0
	return nil
}

func (t *Task) MarkContinuing(summary string) error {
	if t == nil {
		return fmt.Errorf("task: mark continuing: nil task")
	}
	t.Reply = ""
	t.TrajectorySummary = summary
	if t.Phase == "" || t.Phase == TaskPhaseSynthesis {
		t.Phase = TaskPhaseExecution
	}
	t.ReplanCount = 0
	return nil
}

func (t *Task) MarkReplanning(summary string) error {
	if t == nil {
		return fmt.Errorf("task: mark replanning: nil task")
	}
	t.Reply = ""
	t.TrajectorySummary = summary
	t.Phase = TaskPhaseExecution
	t.ReplanCount++
	return nil
}

// TruncatePlanFromStep adjusts plan for replanning. Two modes:
//   - failedStepID non-empty (StepEvaluator): remove that step and all following; keep prefix. Empty prefix clears plan and resets RouteSnap to entry.
//   - failedStepID empty (TrajectoryEvaluator): discard the entire plan and reset RouteSnap to entry (full replan). StepEvaluator must pass a concrete step id.
func (t *Task) TruncatePlanFromStep(failedStepID string) error {
	if t == nil {
		return fmt.Errorf("task: truncate plan: nil task")
	}
	if t.Plan == nil {
		t.Plan = NewPlan()
		if failedStepID == "" {
			return nil
		}
		return fmt.Errorf("task: cannot truncate plan (failed step %q)", failedStepID)
	}
	if err := t.Plan.TruncateFromStep(failedStepID); err != nil {
		if failedStepID == "" {
			return nil
		}
		return fmt.Errorf("task: %w", err)
	}
	return nil
}

// UserFacingTurnMessages returns user-facing chat bubbles.
// TrajectorySummary is internal planning/evaluation context and should not be shown as a normal reply.
func (t *Task) UserFacingTurnMessages() ([]string, error) {
	reply := strings.ReplaceAll(t.Reply, `\n`, "\n")
	summary := strings.ReplaceAll(t.TrajectorySummary, `\n`, "\n")
	r := strings.TrimSpace(reply)
	s := strings.TrimSpace(summary)
	if r == "" && s == "" {
		return nil, fmt.Errorf("task has empty reply")
	}
	var out []string
	if r != "" {
		out = append(out, reply)
	}
	if r == "" && s != "" {
		out = append(out, summary)
	}
	return out, nil
}
