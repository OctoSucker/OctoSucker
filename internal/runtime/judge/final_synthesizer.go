package judge

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	types "github.com/OctoSucker/octosucker/internal/runtime/model"
	"github.com/OctoSucker/octosucker/pkg/llmclient"
)

type FinalSynthesizer struct {
	LLM        *llmclient.OpenAI
	ProjectCtx string
}

func (s *FinalSynthesizer) Synthesize(ctx context.Context, task *types.Task, verdict trajectoryVerdict) (string, error) {
	if task == nil {
		return "", fmt.Errorf("final synthesizer: task is nil")
	}
	if answer, ok := deterministicFinalAnswer(task, verdict); ok {
		return answer, nil
	}
	if s == nil || s.LLM == nil {
		return fallbackFinalAnswer(task, verdict)
	}
	system, user := s.buildFinalAnswerPrompt(task, verdict)
	answer, err := s.LLM.Complete(ctx, system, user)
	if err != nil {
		fallback, ferr := fallbackFinalAnswer(task, verdict)
		if ferr != nil {
			return "", err
		}
		return fallback, nil
	}
	answer = strings.TrimSpace(stripMarkdownFence(answer))
	if answer == "" {
		return fallbackFinalAnswer(task, verdict)
	}
	if formatted, ok := humanizeJSONAnswer(task.UserInput, answer); ok {
		return formatted, nil
	}
	return answer, nil
}

func fallbackFinalAnswer(task *types.Task, verdict trajectoryVerdict) (string, error) {
	if verdict.Outcome == outcomeAbort {
		if strings.TrimSpace(verdict.Rationale) != "" {
			return strings.TrimSpace(verdict.Rationale), nil
		}
		return "当前请求无法继续完成。", nil
	}
	if task == nil || task.Plan == nil {
		return "", fmt.Errorf("final answer fallback: no plan")
	}
	reply, err := task.Plan.UserReply()
	if err != nil {
		return "", err
	}
	if formatted, ok := humanizeJSONAnswer(task.UserInput, reply); ok {
		return formatted, nil
	}
	return reply, nil
}

func deterministicFinalAnswer(task *types.Task, verdict trajectoryVerdict) (string, bool) {
	if verdict.Outcome != outcomeComplete || task == nil || task.Plan == nil {
		return "", false
	}
	if doneStepCount(task.Plan) != 1 {
		return "", false
	}
	step := task.Plan.LastDoneStep()
	if step == nil {
		return "", false
	}
	reply := strings.TrimSpace(step.PrimaryText())
	if reply == "" {
		return "", false
	}
	if formatted, ok := humanizeJSONAnswer(task.UserInput, reply); ok {
		return formatted, true
	}
	return reply, true
}

func doneStepCount(plan *types.Plan) int {
	if plan == nil {
		return 0
	}
	var n int
	for _, step := range plan.Steps {
		if step != nil && step.Status == types.StepStatusDone {
			n++
		}
	}
	return n
}

func buildFinalAnswerPrompt(task *types.Task, verdict trajectoryVerdict) (string, string) {
	return (&FinalSynthesizer{}).buildFinalAnswerPrompt(task, verdict)
}

func (s *FinalSynthesizer) buildFinalAnswerPrompt(task *types.Task, verdict trajectoryVerdict) (string, string) {
	system := `You write the final user-facing answer for an AI agent.

Rules:
- Use the same language as the user unless tool output must remain literal.
- Synthesize across all useful executed steps, not only the last step.
- Do not mention internal trajectory, planner, evaluator, learning, step ids, or implementation details unless the user asked for internals.
- Preserve exact ids, names, file paths, command output, and tool fields when relevant.
- Treat ACCUMULATED EVIDENCE as the concise source of what the agent learned this turn; use raw step output only to recover exact details.
- Prefer concise Markdown bullets for lists. Do not wrap the whole answer in a code fence unless the user asked for code or raw JSON.
- When tool output is structured JSON, translate it into a readable answer. Do not return raw JSON unless the user explicitly requested JSON.
- If outcome is abort, explain the limitation plainly and include the most useful next step.`

	trajectory, err := task.Plan.FormatForPlannerPrompt()
	if err != nil {
		trajectory = fmt.Sprintf("(could not render trajectory: %v)", err)
	}
	evidence := strings.TrimSpace(task.EvidenceSummary)
	if evidence == "" {
		evidence = "(none)"
	}
	project := ""
	if s != nil {
		project = strings.TrimSpace(s.ProjectCtx)
	}
	if project == "" {
		project = "(none)"
	}
	user := fmt.Sprintf(`USER REQUEST:
%s

PROJECT INSTRUCTIONS:
%s

OUTCOME:
%s

TASK PHASE:
%s

JUDGE RATIONALE:
%s

ACCUMULATED EVIDENCE:
%s

EXECUTED STEPS AND OBSERVATIONS:
%s

Write only the final answer the user should see.`, task.UserInput, project, verdict.Outcome, task.Phase, verdict.Rationale, evidence, trajectory)
	return system, user
}

func stripMarkdownFence(s string) string {
	t := strings.TrimSpace(s)
	t = strings.TrimPrefix(t, "```markdown")
	t = strings.TrimPrefix(t, "```md")
	t = strings.TrimPrefix(t, "```")
	t = strings.TrimSuffix(t, "```")
	return strings.TrimSpace(t)
}

func humanizeJSONAnswer(userInput, answer string) (string, bool) {
	if wantsRawJSON(userInput) {
		return "", false
	}
	var obj any
	if err := json.Unmarshal([]byte(strings.TrimSpace(answer)), &obj); err != nil {
		return "", false
	}
	switch v := obj.(type) {
	case map[string]any:
		return humanizeMapList(v)
	case []any:
		return humanizeItems("结果", v)
	default:
		return "", false
	}
}

func wantsRawJSON(userInput string) bool {
	s := strings.ToLower(userInput)
	for _, marker := range []string{"json", "原始", "raw"} {
		if strings.Contains(s, marker) {
			return true
		}
	}
	return false
}

func humanizeMapList(m map[string]any) (string, bool) {
	if tools, ok := m["tools"].([]any); ok {
		provider := humanScalar(m["provider"])
		label := "tools"
		if provider != "" {
			label = provider + " tools"
		}
		return humanizeItems(label, tools)
	}
	if len(m) != 1 {
		return humanizeScalarMap(m)
	}
	for key, val := range m {
		items, ok := val.([]any)
		if !ok {
			return humanizeScalarMap(m)
		}
		if key == "relations" {
			return humanizeRelations(items)
		}
		return humanizeItems(key, items)
	}
	return "", false
}

func humanizeScalarMap(m map[string]any) (string, bool) {
	if len(m) == 0 {
		return "", false
	}
	var lines []string
	for _, key := range sortedHumanKeys(m) {
		if val := humanScalar(m[key]); val != "" {
			lines = append(lines, fmt.Sprintf("- %s: %s", key, val))
		}
	}
	if len(lines) == 0 {
		return "", false
	}
	return strings.Join(lines, "\n"), true
}

func humanizeRelations(items []any) (string, bool) {
	if len(items) == 0 {
		return "没有提取到实体关系。", true
	}
	var b strings.Builder
	b.WriteString("提取到的实体关系：")
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			fmt.Fprintf(&b, "\n- %v", item)
			continue
		}
		from := humanScalar(m["from_id"])
		to := humanScalar(m["to_id"])
		sign := "正相关"
		if val, ok := m["positive"].(bool); ok && !val {
			sign = "负相关"
		}
		if from == "" || to == "" {
			fmt.Fprintf(&b, "\n- %s", humanizeMapItem(m))
			continue
		}
		fmt.Fprintf(&b, "\n- %s -> %s（%s）", from, to, sign)
	}
	return b.String(), true
}

func humanizeItems(label string, items []any) (string, bool) {
	if len(items) == 0 {
		return fmt.Sprintf("没有找到可用的 %s。", label), true
	}
	var b strings.Builder
	fmt.Fprintf(&b, "当前可用的 %s：", label)
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			fmt.Fprintf(&b, "\n- %v", item)
			continue
		}
		fmt.Fprintf(&b, "\n- %s", humanizeMapItem(m))
	}
	return b.String(), true
}

func humanizeMapItem(m map[string]any) string {
	title := firstString(m, "name", "id", "source_file", "provider", "tool")
	if title == "" {
		title = "item"
	}
	var details []string
	for _, key := range sortedHumanKeys(m) {
		if isTitleKey(key) {
			continue
		}
		if val := humanScalar(m[key]); val != "" {
			details = append(details, fmt.Sprintf("%s: %s", key, val))
		}
	}
	if len(details) == 0 {
		return title
	}
	return fmt.Sprintf("%s (%s)", title, strings.Join(details, ", "))
}

func firstString(m map[string]any, keys ...string) string {
	for _, key := range keys {
		if val := humanScalar(m[key]); val != "" {
			return val
		}
	}
	return ""
}

func sortedHumanKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func isTitleKey(key string) bool {
	switch key {
	case "name", "id", "provider", "tool":
		return true
	default:
		return false
	}
}

func humanScalar(v any) string {
	switch x := v.(type) {
	case string:
		return strings.TrimSpace(x)
	case float64:
		return fmt.Sprintf("%v", x)
	case bool:
		if x {
			return "true"
		}
		return "false"
	default:
		return ""
	}
}
