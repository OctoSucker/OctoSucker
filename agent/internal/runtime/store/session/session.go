package session

import (
	"database/sql"
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
	payload, err := marshalSession(sess)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.m[sess.ID] = sess
	s.mu.Unlock()
	return s.persistPut(sess.ID, payload)
}
