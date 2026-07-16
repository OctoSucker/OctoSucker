package model

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/OctoSucker/octosucker/internal/toolcontract"
)

type TurnStatus string

const (
	TurnRunning       TurnStatus = "running"
	TurnCompleted     TurnStatus = "completed"
	TurnAwaitingInput TurnStatus = "awaiting_input"
	TurnBlocked       TurnStatus = "blocked"
)

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type DecisionKind string

const (
	DecisionAct     DecisionKind = "act"
	DecisionRespond DecisionKind = "respond"
)

type Decision struct {
	Kind        DecisionKind        `json:"kind"`
	Disposition ResponseDisposition `json:"disposition,omitempty"`
	Action      Action              `json:"action,omitempty"`
	Step        DecisionStep        `json:"step,omitempty"`
	Reason      string              `json:"reason,omitempty"`
}

type DecisionStep struct {
	Title   string `json:"title"`
	Summary string `json:"summary,omitempty"`
}

type ResponseDisposition string

const (
	ResponseAnswer  ResponseDisposition = "answer"
	ResponseClarify ResponseDisposition = "clarify"
	ResponseBlocked ResponseDisposition = "blocked"
)

type Action struct {
	ID        string         `json:"id"`
	Goal      string         `json:"goal"`
	Tool      string         `json:"tool"`
	Arguments map[string]any `json:"arguments,omitempty"`
}

func (a Action) Fingerprint() string {
	value := struct {
		Tool      string         `json:"tool"`
		Arguments map[string]any `json:"arguments"`
	}{Tool: strings.TrimSpace(a.Tool), Arguments: a.Arguments}
	b, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("%s:%v", value.Tool, value.Arguments)
	}
	return string(b)
}

type Observation struct {
	Result toolcontract.Result `json:"-"`
	Policy toolcontract.Policy `json:"policy,omitempty"`
}

type Progress string

const (
	ProgressContinue Progress = "continue"
	ProgressComplete Progress = "complete"
	ProgressBlocked  Progress = "blocked"
)

type RoutingOutcome string

const (
	RoutingHelpful    RoutingOutcome = "helpful"
	RoutingWrongRoute RoutingOutcome = "wrong_route"
	RoutingNoSignal   RoutingOutcome = "no_signal"
)

type RoutingReason string

const (
	RoutingReasonGoalSatisfied         RoutingReason = "goal_satisfied"
	RoutingReasonNecessaryPrerequisite RoutingReason = "necessary_prerequisite"
	RoutingReasonIrrelevantResult      RoutingReason = "irrelevant_result"
	RoutingReasonWrongStrategy         RoutingReason = "wrong_strategy"
	RoutingReasonValidEmpty            RoutingReason = "valid_empty"
	RoutingReasonAmbiguousResult       RoutingReason = "ambiguous_result"
	RoutingReasonTechnicalError        RoutingReason = "technical_error"
	RoutingReasonEvaluationUnavailable RoutingReason = "evaluation_unavailable"
)

type Assessment struct {
	Progress       Progress       `json:"progress"`
	RoutingOutcome RoutingOutcome `json:"routing_outcome"`
	RoutingReason  RoutingReason  `json:"routing_reason"`
	Summary        string         `json:"summary"`
	NextStepHint   string         `json:"next_step_hint,omitempty"`
}

type Step struct {
	Kind        string      `json:"kind"`
	Title       string      `json:"title"`
	Summary     string      `json:"summary,omitempty"`
	Action      Action      `json:"action"`
	Observation Observation `json:"observation"`
	Assessment  Assessment  `json:"assessment"`
	Status      string      `json:"status,omitempty"`
	StartedAt   time.Time   `json:"started_at,omitempty"`
	CompletedAt *time.Time  `json:"completed_at,omitempty"`
}

// Turn is the aggregate for one user request. Steps are append-only: a new
// planner decision never deletes evidence from an earlier attempt.
type Turn struct {
	ID                  string
	ConversationID      string
	Goal                string
	ConversationContext []Message
	ContextArtifacts    []toolcontract.ContextArtifact
	Steps               []*Step
	Status              TurnStatus
	TerminalReason      string
	Answer              string
	ConsecutiveFailures int
	Trace               []string
	OnStepChanged       func(*Step)
}

func NewTurn(id, conversationID, goal string, context []Message, artifacts ...[]toolcontract.ContextArtifact) *Turn {
	turn := &Turn{
		ID:                  strings.TrimSpace(id),
		ConversationID:      strings.TrimSpace(conversationID),
		Goal:                strings.TrimSpace(goal),
		ConversationContext: append([]Message(nil), context...),
		ContextArtifacts:    make([]toolcontract.ContextArtifact, 0),
		Steps:               make([]*Step, 0),
		Status:              TurnRunning,
	}
	if len(artifacts) > 0 {
		_ = turn.ApplyContextArtifacts(artifacts[0])
	}
	return turn
}

const (
	maxContextArtifacts     = 8
	maxContextArtifactRunes = 20000
	maxContextTotalRunes    = 60000
)

// ApplyContextArtifacts adds trusted, durable context emitted by tools. It is
// kept outside Steps so trajectory truncation cannot silently remove it.
func (t *Turn) ApplyContextArtifacts(artifacts []toolcontract.ContextArtifact) error {
	if t == nil || len(artifacts) == 0 {
		return nil
	}
	for _, artifact := range artifacts {
		artifact.Kind = strings.TrimSpace(artifact.Kind)
		artifact.Name = strings.TrimSpace(artifact.Name)
		artifact.Content = strings.TrimSpace(artifact.Content)
		artifact.Source = strings.TrimSpace(artifact.Source)
		artifact.Digest = strings.TrimSpace(artifact.Digest)
		artifact.Trust = strings.TrimSpace(artifact.Trust)
		if artifact.Kind == "" || artifact.Name == "" || artifact.Content == "" {
			return fmt.Errorf("context artifact requires kind, name, and content")
		}
		if artifact.Trust != toolcontract.TrustWorkspaceInstruction && artifact.Trust != toolcontract.TrustRuntimeMetadata {
			return fmt.Errorf("context artifact %s/%s has unsupported trust %q", artifact.Kind, artifact.Name, artifact.Trust)
		}
		if len([]rune(artifact.Content)) > maxContextArtifactRunes {
			return fmt.Errorf("context artifact %s/%s exceeds %d runes", artifact.Kind, artifact.Name, maxContextArtifactRunes)
		}
		index := -1
		for i := range t.ContextArtifacts {
			if t.ContextArtifacts[i].Kind == artifact.Kind && t.ContextArtifacts[i].Name == artifact.Name {
				index = i
				break
			}
		}
		candidate := append([]toolcontract.ContextArtifact(nil), t.ContextArtifacts...)
		if index >= 0 {
			candidate[index] = artifact
		} else {
			if len(candidate) >= maxContextArtifacts {
				return fmt.Errorf("active context artifact limit reached (%d)", maxContextArtifacts)
			}
			candidate = append(candidate, artifact)
		}
		total := 0
		for _, item := range candidate {
			total += len([]rune(item.Content))
		}
		if total > maxContextTotalRunes {
			return fmt.Errorf("active context exceeds %d runes", maxContextTotalRunes)
		}
		t.ContextArtifacts = candidate
	}
	return nil
}

func (t *Turn) ContextArtifactsSnapshot() []toolcontract.ContextArtifact {
	if t == nil {
		return nil
	}
	return append([]toolcontract.ContextArtifact(nil), t.ContextArtifacts...)
}

func (t *Turn) AppendStep(action Action, observation Observation) *Step {
	step := t.BeginStep(action)
	step.Observation = observation
	t.CompleteStep(step, observation.Result.Err)
	return step
}

func (t *Turn) BeginStep(action Action) *Step {
	step := &Step{Kind: "tool", Title: strings.TrimSpace(action.Goal), Action: action, Status: "running", StartedAt: time.Now().UTC()}
	t.Steps = append(t.Steps, step)
	t.notifyStep(step)
	return step
}

func (t *Turn) BeginResponseStep(description DecisionStep, disposition ResponseDisposition) *Step {
	kind := "response"
	if disposition == ResponseClarify {
		kind = "interaction"
	}
	step := &Step{
		Kind:      kind,
		Title:     strings.TrimSpace(description.Title),
		Summary:   strings.TrimSpace(description.Summary),
		Status:    "running",
		StartedAt: time.Now().UTC(),
	}
	t.Steps = append(t.Steps, step)
	t.notifyStep(step)
	return step
}

func (t *Turn) FinishResponseStep(step *Step, disposition ResponseDisposition, err error) {
	if t == nil || step == nil {
		return
	}
	if err != nil || disposition == ResponseBlocked {
		t.CompleteStep(step, fmt.Errorf("response blocked"))
		return
	}
	if disposition == ResponseClarify {
		step.Status = "waiting_input"
		t.notifyStep(step)
		return
	}
	t.CompleteStep(step, nil)
}

func (t *Turn) CompleteStep(step *Step, err error) {
	if t == nil || step == nil {
		return
	}
	completedAt := time.Now().UTC()
	step.CompletedAt = &completedAt
	if err != nil {
		step.Status = "failed"
	} else {
		step.Status = "completed"
	}
	t.notifyStep(step)
}

func (t *Turn) NotifyStep(step *Step) {
	t.notifyStep(step)
}

func (t *Turn) notifyStep(step *Step) {
	if t != nil && t.OnStepChanged != nil && step != nil {
		t.OnStepChanged(step)
	}
}

func (t *Turn) LastStep() *Step {
	if t == nil || len(t.Steps) == 0 {
		return nil
	}
	return t.Steps[len(t.Steps)-1]
}

func (t *Turn) LastTool() string {
	if step := t.LastStep(); step != nil {
		return step.Action.Tool
	}
	return ""
}

func (t *Turn) HasFailedAction(action Action) bool {
	if t == nil {
		return false
	}
	want := action.Fingerprint()
	for _, step := range t.Steps {
		if step == nil || step.Assessment.RoutingOutcome != RoutingWrongRoute {
			continue
		}
		if step.Action.Fingerprint() == want {
			return true
		}
	}
	return false
}

func (t *Turn) Complete(answer, reason string) {
	t.Answer = strings.TrimSpace(answer)
	t.TerminalReason = strings.TrimSpace(reason)
	t.Status = TurnCompleted
}

func (t *Turn) Block(answer, reason string) {
	t.Answer = strings.TrimSpace(answer)
	t.TerminalReason = strings.TrimSpace(reason)
	t.Status = TurnBlocked
}

func (t *Turn) AwaitInput(answer, reason string) {
	t.Answer = strings.TrimSpace(answer)
	t.TerminalReason = strings.TrimSpace(reason)
	t.Status = TurnAwaitingInput
}

func (t *Turn) AppendTrace(format string, args ...any) {
	if t == nil {
		return
	}
	line := strings.TrimSpace(fmt.Sprintf(format, args...))
	if line == "" {
		return
	}
	t.Trace = append(t.Trace, clipRunes(line, 700))
	if len(t.Trace) > 100 {
		t.Trace = t.Trace[len(t.Trace)-100:]
	}
}

func (t *Turn) TraceSummary() string {
	if t == nil {
		return ""
	}
	return strings.Join(t.Trace, "\n")
}

func (t *Turn) UserFacingMessages() ([]string, error) {
	if t == nil || strings.TrimSpace(t.Answer) == "" {
		return nil, fmt.Errorf("turn has empty answer")
	}
	return []string{strings.ReplaceAll(t.Answer, `\n`, "\n")}, nil
}

func clipRunes(s string, maxRunes int) string {
	runes := []rune(strings.TrimSpace(s))
	if maxRunes <= 0 || len(runes) <= maxRunes {
		return string(runes)
	}
	return string(runes[:maxRunes]) + "..."
}
