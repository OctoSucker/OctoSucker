// Package conversation keeps bounded, process-local dialogue context. It is an
// ingress-independent concern: callers choose the conversation id.
package conversation

import (
	"strings"
	"sync"

	"github.com/OctoSucker/octosucker/internal/runtime/model"
	"github.com/OctoSucker/octosucker/internal/toolcontract"
)

const (
	maxMessages          = 20
	maxConversationRunes = 24000
)

type Store struct {
	mu        sync.RWMutex
	threads   map[string][]model.Message
	artifacts map[string][]toolcontract.ContextArtifact
}

func NewStore() *Store {
	return &Store{
		threads:   make(map[string][]model.Message),
		artifacts: make(map[string][]toolcontract.ContextArtifact),
	}
}

func (s *Store) Context(conversationID string) []model.Message {
	if s == nil {
		return nil
	}
	id := normalizeID(conversationID)
	s.mu.RLock()
	messages := append([]model.Message(nil), s.threads[id]...)
	s.mu.RUnlock()
	return messages
}

func (s *Store) ContextArtifacts(conversationID string) []toolcontract.ContextArtifact {
	if s == nil {
		return nil
	}
	id := normalizeID(conversationID)
	s.mu.RLock()
	artifacts := append([]toolcontract.ContextArtifact(nil), s.artifacts[id]...)
	s.mu.RUnlock()
	return artifacts
}

func (s *Store) RememberContextArtifacts(conversationID string, artifacts []toolcontract.ContextArtifact) {
	if s == nil {
		return
	}
	id := normalizeID(conversationID)
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(artifacts) == 0 {
		delete(s.artifacts, id)
		return
	}
	s.artifacts[id] = append([]toolcontract.ContextArtifact(nil), artifacts...)
}

func (s *Store) AppendExchange(conversationID, user, assistant string) {
	if s == nil {
		return
	}
	id := normalizeID(conversationID)
	user = strings.TrimSpace(user)
	assistant = strings.TrimSpace(assistant)
	if user == "" && assistant == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	messages := s.threads[id]
	if user != "" {
		messages = append(messages, model.Message{Role: "user", Content: user})
	}
	if assistant != "" {
		messages = append(messages, model.Message{Role: "assistant", Content: assistant})
	}
	s.threads[id] = bounded(messages)
}

func bounded(messages []model.Message) []model.Message {
	if len(messages) > maxMessages {
		messages = messages[len(messages)-maxMessages:]
	}
	total := 0
	start := len(messages)
	for i := len(messages) - 1; i >= 0; i-- {
		total += len([]rune(messages[i].Content))
		if total > maxConversationRunes {
			break
		}
		start = i
	}
	return append([]model.Message(nil), messages[start:]...)
}

func normalizeID(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return "default"
	}
	return id
}
