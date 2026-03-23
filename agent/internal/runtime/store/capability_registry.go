package store

import (
	"sync"

	"github.com/OctoSucker/agent/pkg/ports"
)

type CapabilityRegistry struct {
	mu   sync.RWMutex
	caps map[string]ports.Capability
}

func NewCapabilityRegistryFromCapabilities(m map[string]ports.Capability) *CapabilityRegistry {
	r := &CapabilityRegistry{caps: make(map[string]ports.Capability)}
	for _, c := range m {
		r.Register(c)
	}
	return r
}

func (r *CapabilityRegistry) Register(c ports.Capability) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.caps[c.ID] = c
}

func (r *CapabilityRegistry) FirstTool(capID string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.caps[capID]
	if !ok || len(c.Tools) == 0 {
		return ""
	}
	return c.Tools[0]
}

func (r *CapabilityRegistry) Tools(capID string) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.caps[capID]
	if !ok {
		return nil
	}
	out := make([]string, len(c.Tools))
	copy(out, c.Tools)
	return out
}
