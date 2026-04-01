// Package task is the task repository: in-memory cache with optional SQLite persistence.
package task

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/OctoSucker/agent/model"
	"github.com/OctoSucker/agent/pkg/ports"
)

type TaskStore struct {
	mu    sync.RWMutex
	m     map[string]*ports.Task
	store *model.AgentDB
}

func NewTaskStore(store *model.AgentDB) (*TaskStore, error) {
	s := &TaskStore{m: make(map[string]*ports.Task), store: store}
	if store != nil {
		if err := s.loadAll(); err != nil {
			return nil, err
		}
	}
	return s, nil
}

func (s *TaskStore) Get(id string) (*ports.Task, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.m[id]
	return v, ok
}

// GetOrCreate returns the task for id, or allocates a new in-memory *ports.Task and inserts it (not persisted until Put).
func (s *TaskStore) GetOrCreate(id string) (*ports.Task, error) {
	if id == "" {
		return nil, fmt.Errorf("task store: empty id")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if v, ok := s.m[id]; ok {
		return v, nil
	}
	t := &ports.Task{
		ID: id,
		Plan: &ports.Plan{
			Steps: make([]*ports.PlanStep, 0),
		},
	}
	s.m[id] = t
	return t, nil
}

func (s *TaskStore) Put(t *ports.Task) error {
	if t == nil {
		return fmt.Errorf("task store: nil task")
	}
	if t.ID == "" {
		return fmt.Errorf("task store: task has empty id")
	}
	payload, err := marshalTask(t)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.m[t.ID] = t
	s.mu.Unlock()
	return s.persistPut(t.ID, payload)
}

func marshalTask(t *ports.Task) ([]byte, error) {
	b, err := json.Marshal(t)
	if err != nil {
		return nil, fmt.Errorf("task store put: marshal: %w", err)
	}
	return b, nil
}

func (s *TaskStore) loadAll() error {
	rows, err := s.store.TaskSelectAll()
	if err != nil {
		return fmt.Errorf("task store load: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, r := range rows {
		var t ports.Task
		if err := json.Unmarshal([]byte(r.Payload), &t); err != nil {
			continue
		}
		if t.ID == "" {
			t.ID = r.ID
		}
		cp := t
		s.m[t.ID] = &cp
	}
	return nil
}

func (s *TaskStore) persistPut(id string, payload []byte) error {
	if s.store == nil {
		return nil
	}
	if err := s.store.TaskUpsert(id, string(payload)); err != nil {
		return fmt.Errorf("task store put: db: %w", err)
	}
	return nil
}
