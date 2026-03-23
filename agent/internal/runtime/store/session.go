package store

import (
	"sync"

	"github.com/OctoSucker/agent/pkg/ports"
)

type SessionStore struct {
	mu sync.RWMutex
	m  map[string]*ports.Session
}

func NewSessionStore() *SessionStore {
	return &SessionStore{m: make(map[string]*ports.Session)}
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
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[sess.ID] = sess
	return nil
}
