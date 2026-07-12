// Package contextmanager builds bounded, role-specific prompt context from a
// turn. It owns context selection and rendering, but never plans, evaluates,
// invokes tools, or calls an LLM.
package contextmanager

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/OctoSucker/octosucker/internal/runtime/model"
	"github.com/OctoSucker/octosucker/internal/toolcontract"
)

type Audience string

const (
	AudiencePlanner   Audience = "planner"
	AudienceEvaluator Audience = "evaluator"
	AudienceResponder Audience = "responder"
)

type Limits struct {
	PlannerTokens   int
	EvaluatorTokens int
	ResponderTokens int
}

type Input struct {
	Turn                *model.Turn
	ProjectInstructions string
	Tools               []toolcontract.ToolDescriptor
	Skills              []map[string]any
	RoutingHints        []string
	ValidationError     string
}

type Stats struct {
	Audience         Audience
	BudgetTokens     int
	EstimatedTokens  int
	OmittedMessages  int
	OmittedSteps     int
	OmittedTools     int
	OmittedArtifacts int
}

func (s Stats) Trace() string {
	return fmt.Sprintf("context audience=%s tokens=%d/%d omitted_messages=%d omitted_steps=%d omitted_tools=%d omitted_artifacts=%d",
		s.Audience, s.EstimatedTokens, s.BudgetTokens, s.OmittedMessages, s.OmittedSteps, s.OmittedTools, s.OmittedArtifacts)
}

type Snapshot struct {
	Goal                string
	ProjectInstructions string
	Conversation        string
	Skills              string
	ActiveInstructions  string
	Tools               string
	RoutingHints        string
	Trajectory          string
	ValidationError     string
	Stats               Stats
}

type Manager struct {
	limits Limits
}

func New(limits Limits) *Manager {
	limits.PlannerTokens = positiveOr(limits.PlannerTokens, 64000)
	limits.EvaluatorTokens = positiveOr(limits.EvaluatorTokens, 48000)
	limits.ResponderTokens = positiveOr(limits.ResponderTokens, 64000)
	return &Manager{limits: limits}
}

func (m *Manager) Build(audience Audience, in Input) Snapshot {
	if m == nil {
		m = New(Limits{})
	}
	budget := m.budget(audience)
	allocation := allocationFor(audience, budget)
	turn := in.Turn
	if turn == nil {
		return Snapshot{Stats: Stats{Audience: audience, BudgetTokens: budget}}
	}

	conversation, omittedMessages := renderConversation(turn.ConversationContext, allocation.conversation)
	active, omittedArtifacts := renderArtifacts(turn.ContextArtifacts, turn.Goal, allocation.active)
	trajectory, omittedSteps := renderTrajectory(turn.Steps, allocation.trajectory)
	tools, omittedTools := renderRankedJSON(in.Tools, allocation.tools, func(tool toolcontract.ToolDescriptor) int {
		return toolScore(tool, turn, in.RoutingHints)
	})
	if omittedTools > 0 {
		tools = fmt.Sprintf("[%d least relevant tools omitted by the context budget; do not invent their schemas]\n%s", omittedTools, tools)
	}

	snapshot := Snapshot{
		Goal:                clipText(turn.Goal, allocation.goal),
		ProjectInstructions: noneIfEmpty(clipText(in.ProjectInstructions, allocation.project)),
		Conversation:        noneIfEmpty(conversation),
		Skills:              noneIfEmpty(renderJSON(in.Skills, allocation.skills)),
		ActiveInstructions:  noneIfEmpty(active),
		Tools:               noneIfEmpty(tools),
		RoutingHints:        noneIfEmpty(renderJSON(in.RoutingHints, allocation.routing)),
		Trajectory:          noneIfEmpty(trajectory),
		ValidationError:     noneIfEmpty(clipText(in.ValidationError, allocation.feedback)),
		Stats: Stats{
			Audience:         audience,
			BudgetTokens:     budget,
			OmittedMessages:  omittedMessages,
			OmittedSteps:     omittedSteps,
			OmittedTools:     omittedTools,
			OmittedArtifacts: omittedArtifacts,
		},
	}
	snapshot.Stats.EstimatedTokens = estimateTokens(strings.Join([]string{
		snapshot.Goal, snapshot.ProjectInstructions, snapshot.Conversation,
		snapshot.Skills, snapshot.ActiveInstructions, snapshot.Tools,
		snapshot.RoutingHints, snapshot.Trajectory, snapshot.ValidationError,
	}, "\n"))
	return snapshot
}

func (m *Manager) budget(audience Audience) int {
	switch audience {
	case AudienceEvaluator:
		return m.limits.EvaluatorTokens
	case AudienceResponder:
		return m.limits.ResponderTokens
	default:
		return m.limits.PlannerTokens
	}
}

type allocation struct {
	goal, project, conversation, skills, active, tools, routing, trajectory, feedback int
}

func allocationFor(audience Audience, total int) allocation {
	pct := func(n int) int { return max(64, total*n/100) }
	switch audience {
	case AudienceEvaluator:
		return allocation{goal: pct(5), project: pct(8), conversation: pct(12), active: pct(20), trajectory: pct(50), feedback: pct(3)}
	case AudienceResponder:
		return allocation{goal: pct(5), project: pct(8), conversation: pct(15), skills: pct(5), active: pct(15), tools: pct(14), trajectory: pct(35)}
	default:
		return allocation{goal: pct(4), project: pct(6), conversation: pct(12), skills: pct(5), active: pct(14), tools: pct(27), routing: pct(2), trajectory: pct(27), feedback: pct(2)}
	}
}

func renderConversation(messages []model.Message, budget int) (string, int) {
	if len(messages) == 0 || budget <= 0 {
		return "", len(messages)
	}
	selected := make([]string, 0, len(messages))
	used := 0
	for i := len(messages) - 1; i >= 0; i-- {
		role := strings.ToUpper(strings.TrimSpace(messages[i].Role))
		if role == "" {
			role = "MESSAGE"
		}
		entry := fmt.Sprintf("%s: %s", role, strings.TrimSpace(messages[i].Content))
		entry = clipText(entry, max(128, budget/2))
		cost := estimateTokens(entry) + 2
		if used+cost > budget && len(selected) > 0 {
			break
		}
		selected = append(selected, entry)
		used += cost
	}
	reverse(selected)
	omitted := len(messages) - len(selected)
	if omitted > 0 {
		selected = append([]string{fmt.Sprintf("[%d older conversation messages omitted]", omitted)}, selected...)
	}
	return strings.Join(selected, "\n"), omitted
}

func renderArtifacts(artifacts []toolcontract.ContextArtifact, goal string, budget int) (string, int) {
	if len(artifacts) == 0 || budget <= 0 {
		return "", len(artifacts)
	}
	type ranked struct {
		index int
		score int
		text  string
	}
	items := make([]ranked, 0, len(artifacts))
	for i, artifact := range artifacts {
		var b strings.Builder
		fmt.Fprintf(&b, "--- %s: %s ---\n", artifact.Kind, artifact.Name)
		if artifact.Source != "" {
			fmt.Fprintf(&b, "Source: %s\n", artifact.Source)
		}
		fmt.Fprintf(&b, "Trust: %s\nInstructions:\n%s", artifact.Trust, artifact.Content)
		items = append(items, ranked{index: i, score: overlapScore(goal, artifact.Name+" "+artifact.Content), text: b.String()})
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].score == items[j].score {
			return items[i].index < items[j].index
		}
		return items[i].score > items[j].score
	})
	var selected []string
	used := 0
	for _, item := range items {
		text := clipText(item.text, budget)
		cost := estimateTokens(text) + 2
		if used+cost > budget && len(selected) > 0 {
			continue
		}
		selected = append(selected, text)
		used += cost
	}
	omitted := len(artifacts) - len(selected)
	if omitted > 0 {
		selected = append(selected, fmt.Sprintf("[%d less relevant active context artifacts omitted]", omitted))
	}
	return strings.Join(selected, "\n\n"), omitted
}

func renderTrajectory(steps []*model.Step, budget int) (string, int) {
	if len(steps) == 0 || budget <= 0 {
		return "", len(steps)
	}
	selected := make([]string, 0, len(steps))
	used := 0
	for i := len(steps) - 1; i >= 0; i-- {
		outputBudget := min(6000, max(512, budget/3))
		if i != len(steps)-1 {
			outputBudget = min(2500, max(256, budget/8))
		}
		entry := renderStep(i, steps[i], outputBudget)
		cost := estimateTokens(entry) + 4
		if used+cost > budget && len(selected) > 0 {
			break
		}
		if cost > budget {
			entry = clipText(entry, budget)
			cost = estimateTokens(entry)
		}
		selected = append(selected, entry)
		used += cost
	}
	reverse(selected)
	omitted := len(steps) - len(selected)
	if omitted > 0 {
		var summary strings.Builder
		fmt.Fprintf(&summary, "[%d older steps compacted]\n", omitted)
		for i := 0; i < omitted; i++ {
			step := steps[i]
			if step == nil {
				continue
			}
			fmt.Fprintf(&summary, "- step %d tool=%s progress=%s routing_outcome=%s routing_reason=%s summary=%s\n",
				i+1, step.Action.Tool, step.Assessment.Progress, step.Assessment.RoutingOutcome,
				step.Assessment.RoutingReason, clipText(step.Assessment.Summary, 120))
		}
		selected = append([]string{clipText(summary.String(), max(128, budget-used))}, selected...)
	}
	return strings.Join(selected, "\n\n"), omitted
}

func renderStep(index int, step *model.Step, outputBudget int) string {
	if step == nil {
		return fmt.Sprintf("--- Action %d ---\n(empty step)", index+1)
	}
	args, _ := json.Marshal(step.Action.Arguments)
	result := step.Observation.Result
	var b strings.Builder
	fmt.Fprintf(&b, "--- Action %d ---\n", index+1)
	fmt.Fprintf(&b, "Goal: %s\nTool: %s\nArguments: %s\n", step.Action.Goal, step.Action.Tool, args)
	fmt.Fprintf(&b, "Policy: risk=%s output_trust=%s capabilities=%v summary=%s\n",
		step.Observation.Policy.Risk, step.Observation.Policy.OutputTrust, step.Observation.Policy.Capabilities, step.Observation.Policy.Summary)
	fmt.Fprintf(&b, "Result: kind=%s count=%d empty=%v\n", result.Kind, result.Count, result.Empty)
	if result.Err != nil {
		fmt.Fprintf(&b, "Error: %s\n", result.Err)
	} else if text := strings.TrimSpace(result.CompactText()); text != "" {
		fmt.Fprintf(&b, "Output:\n%s\n", clipText(text, outputBudget))
	}
	if step.Assessment.Progress != "" {
		fmt.Fprintf(&b, "Assessment: progress=%s routing_outcome=%s routing_reason=%s summary=%s next=%s",
			step.Assessment.Progress, step.Assessment.RoutingOutcome, step.Assessment.RoutingReason,
			step.Assessment.Summary, step.Assessment.NextStepHint)
	}
	return strings.TrimSpace(b.String())
}

func renderRankedJSON[T any](items []T, budget int, score func(T) int) (string, int) {
	if len(items) == 0 || budget <= 0 {
		return "", len(items)
	}
	all := prettyJSON(items)
	if estimateTokens(all) <= budget {
		return all, 0
	}
	type ranked struct {
		index int
		score int
		item  T
	}
	rankedItems := make([]ranked, 0, len(items))
	for i, item := range items {
		rankedItems = append(rankedItems, ranked{index: i, score: score(item), item: item})
	}
	sort.SliceStable(rankedItems, func(i, j int) bool {
		if rankedItems[i].score == rankedItems[j].score {
			return rankedItems[i].index < rankedItems[j].index
		}
		return rankedItems[i].score > rankedItems[j].score
	})
	selected := make([]T, 0, len(items))
	for _, item := range rankedItems {
		candidate := append(append([]T(nil), selected...), item.item)
		if estimateTokens(prettyJSON(candidate)) > budget && len(selected) > 0 {
			continue
		}
		selected = candidate
	}
	return prettyJSON(selected), len(items) - len(selected)
}

func renderJSON(v any, budget int) string {
	if budget <= 0 {
		return ""
	}
	return clipText(prettyJSON(v), budget)
}

func toolScore(tool toolcontract.ToolDescriptor, turn *model.Turn, hints []string) int {
	score := overlapScore(turn.Goal, tool.Name+" "+tool.Description+" "+strings.Join(tool.Capabilities, " "))
	if tool.Name == "activate_skill" || tool.Name == "read_skill_resource" {
		score += 100
	}
	if tool.Name == turn.LastTool() {
		score += 80
	}
	for _, hint := range hints {
		if tool.Name == hint {
			score += 60
		}
	}
	for _, step := range turn.Steps {
		if step != nil && step.Action.Tool == tool.Name {
			score += 40
			break
		}
	}
	return score
}

func overlapScore(a, b string) int {
	aw := words(a)
	bw := words(b)
	score := 0
	for word := range aw {
		if _, ok := bw[word]; ok {
			score++
		}
	}
	return score
}

func words(s string) map[string]struct{} {
	out := make(map[string]struct{})
	var latin []rune
	var cjk []rune
	flushLatin := func() {
		if len(latin) >= 2 {
			out[string(latin)] = struct{}{}
		}
		latin = latin[:0]
	}
	flushCJK := func() {
		for i := 0; i < len(cjk); i++ {
			out[string(cjk[i])] = struct{}{}
			if i+1 < len(cjk) {
				out[string(cjk[i:i+2])] = struct{}{}
			}
		}
		cjk = cjk[:0]
	}
	for _, r := range []rune(strings.ToLower(s)) {
		switch {
		case unicode.Is(unicode.Han, r):
			flushLatin()
			cjk = append(cjk, r)
		case unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-':
			flushCJK()
			latin = append(latin, r)
		default:
			flushLatin()
			flushCJK()
		}
	}
	flushLatin()
	flushCJK()
	return out
}

func estimateTokens(s string) int {
	ascii, nonASCII := 0, 0
	for _, r := range s {
		if r <= unicode.MaxASCII {
			ascii++
		} else {
			nonASCII++
		}
	}
	return (ascii+3)/4 + nonASCII
}

func clipText(s string, budget int) string {
	s = strings.TrimSpace(s)
	if s == "" || budget <= 0 || estimateTokens(s) <= budget {
		return s
	}
	runes := []rune(s)
	lo, hi := 0, len(runes)
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if estimateTokens(string(runes[:mid])+"\n[content truncated]") <= budget {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return strings.TrimSpace(string(runes[:lo])) + "\n[content truncated]"
}

func prettyJSON(v any) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(b)
}

func noneIfEmpty(s string) string {
	if strings.TrimSpace(s) == "" {
		return "(none)"
	}
	return s
}

func positiveOr(value, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}

func reverse[T any](items []T) {
	for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
		items[i], items[j] = items[j], items[i]
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
