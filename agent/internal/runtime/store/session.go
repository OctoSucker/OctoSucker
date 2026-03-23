package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/OctoSucker/agent/pkg/ports"
)

type SessionStore struct {
	mu sync.RWMutex
	m  map[string]*ports.Session
	db *sql.DB
}

func NewSessionStore(db *sql.DB) *SessionStore {
	s := &SessionStore{m: make(map[string]*ports.Session), db: db}
	if db != nil {
		_ = s.loadAll()
	}
	return s
}

func (s *SessionStore) loadAll() error {
	rows, err := s.db.Query(`SELECT id, payload FROM sessions`)
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

func (s *SessionStore) Get(id string) (*ports.Session, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.m[id]
	return v, ok
}

func (s *SessionStore) Put(sess *ports.Session) error {
	if sess == nil {
		return nil
	}
	payload, err := json.Marshal(sess)
	if err != nil {
		return fmt.Errorf("sessions put: marshal: %w", err)
	}
	s.mu.Lock()
	s.m[sess.ID] = sess
	s.mu.Unlock()
	if s.db != nil {
		if _, err := s.db.Exec(`INSERT INTO sessions (id, payload) VALUES (?, ?)
			ON CONFLICT(id) DO UPDATE SET payload = excluded.payload`, sess.ID, string(payload)); err != nil {
			return fmt.Errorf("sessions put: db: %w", err)
		}
	}
	return nil
}
