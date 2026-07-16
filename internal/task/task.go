// Package task owns the in-memory, user-facing lifecycle of assistant work.
package task

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/OctoSucker/octosucker/internal/interaction"
	"github.com/google/uuid"
)

type Status string

const (
	StatusRunning         Status = "running"
	StatusWaitingInput    Status = "waiting_input"
	StatusWaitingApproval Status = "waiting_approval"
	StatusCompleted       Status = "completed"
	StatusFailed          Status = "failed"
	StatusCancelled       Status = "cancelled"
)

func (s Status) Terminal() bool {
	return s == StatusCompleted || s == StatusFailed || s == StatusCancelled
}

type Message struct {
	ID        string    `json:"id"`
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

type Step struct {
	ID          string     `json:"id"`
	Sequence    int        `json:"sequence"`
	Kind        string     `json:"kind"`
	Title       string     `json:"title"`
	Tool        string     `json:"tool,omitempty"`
	Status      string     `json:"status"`
	Summary     string     `json:"summary,omitempty"`
	Evaluation  string     `json:"evaluation,omitempty"`
	RetryCount  int        `json:"retry_count,omitempty"`
	StartedAt   time.Time  `json:"started_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	DurationMS  int64      `json:"duration_ms,omitempty"`
}

type Approval struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Status      string `json:"status"`
}

type Result struct {
	Summary string `json:"summary"`
}

type InputResult struct {
	Action string   `json:"action"`
	Task   Snapshot `json:"task"`
}

type Snapshot struct {
	ID                 string                   `json:"id"`
	ParentTaskID       string                   `json:"parent_task_id,omitempty"`
	Title              string                   `json:"title"`
	Status             Status                   `json:"status"`
	Version            uint64                   `json:"version"`
	CreatedAt          time.Time                `json:"created_at"`
	UpdatedAt          time.Time                `json:"updated_at"`
	Steps              []Step                   `json:"steps"`
	Messages           []Message                `json:"messages"`
	PendingInteraction *interaction.Interaction `json:"pending_interaction,omitempty"`
	PendingApproval    *Approval                `json:"pending_approval,omitempty"`
	Result             *Result                  `json:"result,omitempty"`
	Error              string                   `json:"error,omitempty"`
}

type Store struct {
	mu    sync.RWMutex
	items map[string]*Snapshot
}

func NewStore() *Store {
	return &Store{items: make(map[string]*Snapshot)}
}

func (s *Store) Create(input, parentTaskID string) Snapshot {
	now := time.Now().UTC()
	id := uuid.NewString()
	title := clip(strings.TrimSpace(input), 48)
	if title == "" {
		title = "新的任务"
	}
	item := &Snapshot{
		ID:           id,
		ParentTaskID: strings.TrimSpace(parentTaskID),
		Title:        title,
		Status:       StatusRunning,
		Version:      1,
		CreatedAt:    now,
		UpdatedAt:    now,
		Steps:        make([]Step, 0),
		Messages:     []Message{newMessage("user", input)},
	}
	s.mu.Lock()
	s.items[id] = item
	s.mu.Unlock()
	return clone(*item)
}

func (s *Store) Get(id string) (Snapshot, bool) {
	s.mu.RLock()
	item, ok := s.items[strings.TrimSpace(id)]
	if !ok {
		s.mu.RUnlock()
		return Snapshot{}, false
	}
	out := clone(*item)
	s.mu.RUnlock()
	return out, true
}

func (s *Store) PrepareInput(id, content string) (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.items[strings.TrimSpace(id)]
	if !ok {
		return Snapshot{}, fmt.Errorf("task not found")
	}
	if item.Status != StatusWaitingInput {
		return Snapshot{}, fmt.Errorf("task status %s does not accept input", item.Status)
	}
	item.Status = StatusRunning
	item.PendingInteraction = nil
	item.Error = ""
	item.Result = nil
	item.Messages = append(item.Messages, newMessage("user", content))
	for i := len(item.Steps) - 1; i >= 0; i-- {
		if item.Steps[i].Status == "waiting_input" {
			completedAt := time.Now().UTC()
			item.Steps[i].Status = "completed"
			item.Steps[i].CompletedAt = &completedAt
			if item.Steps[i].Summary == "" {
				item.Steps[i].Summary = "已收到用户补充信息"
			}
			if !item.Steps[i].StartedAt.IsZero() {
				item.Steps[i].DurationMS = completedAt.Sub(item.Steps[i].StartedAt).Milliseconds()
			}
			break
		}
	}
	touch(item)
	return clone(*item), nil
}

func (s *Store) UpdateStep(id string, step Step) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.items[id]
	if !ok {
		return
	}
	found := false
	for i := range item.Steps {
		if item.Steps[i].ID == step.ID {
			step.Sequence = item.Steps[i].Sequence
			item.Steps[i] = step
			found = true
			break
		}
	}
	if !found {
		step.Sequence = len(item.Steps) + 1
		item.Steps = append(item.Steps, step)
	}
	touch(item)
}

func (s *Store) RequireApproval(id string, approval Approval) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.items[id]
	if !ok {
		return fmt.Errorf("task not found")
	}
	approval.Status = "pending"
	item.Status = StatusWaitingApproval
	item.PendingApproval = &approval
	for i := len(item.Steps) - 1; i >= 0; i-- {
		if item.Steps[i].Status == "running" {
			item.Steps[i].Status = "waiting_approval"
			break
		}
	}
	touch(item)
	return nil
}

func (s *Store) ResolveApproval(id, approvalID, decision string) (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.items[id]
	if !ok {
		return Snapshot{}, fmt.Errorf("task not found")
	}
	if item.Status != StatusWaitingApproval || item.PendingApproval == nil || item.PendingApproval.ID != approvalID {
		return Snapshot{}, fmt.Errorf("approval does not match the pending task approval")
	}
	if decision != "approved" && decision != "rejected" {
		return Snapshot{}, fmt.Errorf("decision must be approved or rejected")
	}
	item.PendingApproval.Status = decision
	item.Status = StatusRunning
	item.PendingApproval = nil
	for i := len(item.Steps) - 1; i >= 0; i-- {
		if item.Steps[i].Status == "waiting_approval" {
			item.Steps[i].Status = "running"
			break
		}
	}
	touch(item)
	return clone(*item), nil
}

func (s *Store) Finish(id string, status Status, assistantMessages []string, form *interaction.Interaction, resultSummary, errorText string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.items[id]
	if !ok {
		return
	}
	for _, content := range assistantMessages {
		if strings.TrimSpace(content) != "" {
			item.Messages = append(item.Messages, newMessage("assistant", content))
		}
	}
	item.Status = status
	item.PendingInteraction = form
	item.Error = strings.TrimSpace(errorText)
	if status == StatusCompleted {
		item.Result = &Result{Summary: strings.TrimSpace(resultSummary)}
	} else {
		item.Result = nil
	}
	touch(item)
}

func newMessage(role, content string) Message {
	return Message{ID: uuid.NewString(), Role: role, Content: strings.TrimSpace(content), CreatedAt: time.Now().UTC()}
}

func touch(item *Snapshot) {
	item.Version++
	item.UpdatedAt = time.Now().UTC()
}

func clone(in Snapshot) Snapshot {
	out := in
	out.Steps = append(make([]Step, 0, len(in.Steps)), in.Steps...)
	out.Messages = append(make([]Message, 0, len(in.Messages)), in.Messages...)
	if in.PendingInteraction != nil {
		copyInteraction := *in.PendingInteraction
		copyInteraction.Fields = append([]interaction.Field(nil), in.PendingInteraction.Fields...)
		out.PendingInteraction = &copyInteraction
	}
	if in.PendingApproval != nil {
		copyApproval := *in.PendingApproval
		out.PendingApproval = &copyApproval
	}
	if in.Result != nil {
		copyResult := *in.Result
		out.Result = &copyResult
	}
	return out
}

func clip(value string, maxRunes int) string {
	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value
	}
	return string(runes[:maxRunes])
}
