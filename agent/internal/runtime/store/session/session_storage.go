package session

import (
	"encoding/json"
	"fmt"

	"github.com/OctoSucker/agent/pkg/ports"
)

// Must match store/tables.go migrate (TableSessions).
const sqliteTableSessions = "sessions"

func marshalSession(sess *ports.Session) ([]byte, error) {
	b, err := json.Marshal(sess)
	if err != nil {
		return nil, fmt.Errorf("sessions put: marshal: %w", err)
	}
	return b, nil
}

func (s *SessionStore) loadAll() error {
	rows, err := s.db.Query(fmt.Sprintf(`SELECT id, payload FROM %s`, sqliteTableSessions))
	if err != nil {
		return fmt.Errorf("sessions load: %w", err)
	}
	defer rows.Close()
	s.mu.Lock()
	defer s.mu.Unlock()
	for rows.Next() {
		var id, payload string
		if err := rows.Scan(&id, &payload); err != nil {
			return err
		}
		var sess ports.Session
		if err := json.Unmarshal([]byte(payload), &sess); err != nil {
			continue
		}
		if sess.ID == "" {
			sess.ID = id
		}
		cp := sess
		s.m[sess.ID] = &cp
	}
	return rows.Err()
}

func (s *SessionStore) persistPut(id string, payload []byte) error {
	if s.db == nil {
		return nil
	}
	if _, err := s.db.Exec(fmt.Sprintf(`INSERT INTO %s (id, payload) VALUES (?, ?)
			ON CONFLICT(id) DO UPDATE SET payload = excluded.payload`, sqliteTableSessions), id, string(payload)); err != nil {
		return fmt.Errorf("sessions put: db: %w", err)
	}
	return nil
}
