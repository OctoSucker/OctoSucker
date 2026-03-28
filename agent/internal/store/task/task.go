package task

import (
	"database/sql"
	"fmt"
	"sync"

	"github.com/OctoSucker/agent/pkg/ports"
)

type TaskStore struct {
	mu sync.RWMutex
	m  map[string]*ports.Task
	db *sql.DB
}

func NewTaskStore(db *sql.DB) (*TaskStore, error) {
	s := &TaskStore{m: make(map[string]*ports.Task), db: db}
	if db != nil {
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
	t := &ports.Task{ID: id}
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
