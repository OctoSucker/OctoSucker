package task

import (
	"encoding/json"
	"fmt"

	"github.com/OctoSucker/agent/internal/runtime/store"
	"github.com/OctoSucker/agent/pkg/ports"
)

func marshalTask(t *ports.Task) ([]byte, error) {
	b, err := json.Marshal(t)
	if err != nil {
		return nil, fmt.Errorf("task store put: marshal: %w", err)
	}
	return b, nil
}

func (s *TaskStore) loadAll() error {
	rows, err := s.db.Query(fmt.Sprintf(`SELECT id, payload FROM %s`, store.TableTasks))
	if err != nil {
		return fmt.Errorf("task store load: %w", err)
	}
	defer rows.Close()
	s.mu.Lock()
	defer s.mu.Unlock()
	for rows.Next() {
		var id, payload string
		if err := rows.Scan(&id, &payload); err != nil {
			return err
		}
		var t ports.Task
		if err := json.Unmarshal([]byte(payload), &t); err != nil {
			continue
		}
		if t.ID == "" {
			t.ID = id
		}
		cp := t
		s.m[t.ID] = &cp
	}
	return rows.Err()
}

func (s *TaskStore) persistPut(id string, payload []byte) error {
	if s.db == nil {
		return nil
	}
	if _, err := s.db.Exec(fmt.Sprintf(`INSERT INTO %s (id, payload) VALUES (?, ?)
			ON CONFLICT(id) DO UPDATE SET payload = excluded.payload`, store.TableTasks), id, string(payload)); err != nil {
		return fmt.Errorf("task store put: db: %w", err)
	}
	return nil
}
