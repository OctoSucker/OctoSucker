// Package taskstore holds executor task snapshots in memory only (no SQLite persistence).
package taskstore

import (
	"fmt"
	"sync"

	"github.com/OctoSucker/octosucker/engine/types"
)

type TaskStore struct {
	mu sync.RWMutex
	m  map[string]*types.Task
}

func NewTaskStore() *TaskStore {
	return &TaskStore{m: make(map[string]*types.Task)}
}

func (s *TaskStore) Get(id string) (*types.Task, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.m[id]
	return v, ok
}

// GetOrCreate returns the task for id, or allocates a new in-memory *types.Task and inserts it (not persisted until Put).
func (s *TaskStore) GetOrCreate(id string) (*types.Task, error) {
	if id == "" {
		return nil, fmt.Errorf("task store: empty id")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if v, ok := s.m[id]; ok {
		return v, nil
	}
	t := &types.Task{
		ID: id,
		Plan: &types.Plan{
			Steps: make([]*types.PlanStep, 0),
		},
	}
	s.m[id] = t
	return t, nil
}

func (s *TaskStore) Put(t *types.Task) error {
	if t == nil {
		return fmt.Errorf("task store: nil task")
	}
	if t.ID == "" {
		return fmt.Errorf("task store: task has empty id")
	}
	s.mu.Lock()
	s.m[t.ID] = t
	s.mu.Unlock()
	return nil
}
