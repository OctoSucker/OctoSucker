package store

import (
	"database/sql"
	"encoding/json"
	"sync"

	"github.com/OctoSucker/agent/pkg/ports"
)

type CapabilityRegistry struct {
	mu   sync.RWMutex
	caps map[string]ports.Capability
	db   *sql.DB
}

func NewCapabilityRegistryFromCapabilities(m map[string]ports.Capability, db *sql.DB) *CapabilityRegistry {
	r := &CapabilityRegistry{caps: make(map[string]ports.Capability), db: db}
	for _, c := range m {
		r.Register(c)
	}
	if db != nil {
		r.loadOrphansFromDB(m)
	}
	return r
}

func (r *CapabilityRegistry) loadOrphansFromDB(live map[string]ports.Capability) {
	rows, err := r.db.Query(`SELECT id, tools_json FROM capabilities`)
	if err != nil {
		return
	}
	defer rows.Close()
	r.mu.Lock()
	defer r.mu.Unlock()
	for rows.Next() {
		var id, toolsJSON string
		if err := rows.Scan(&id, &toolsJSON); err != nil {
			continue
		}
		if _, ok := live[id]; ok {
			continue
		}
		var tools []string
		if err := json.Unmarshal([]byte(toolsJSON), &tools); err != nil {
			continue
		}
		r.caps[id] = ports.Capability{ID: id, Tools: tools}
	}
}

func (r *CapabilityRegistry) Register(c ports.Capability) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.caps[c.ID] = c
	if r.db == nil {
		return
	}
	toolsJSON, err := json.Marshal(c.Tools)
	if err != nil {
		return
	}
	_, _ = r.db.Exec(`INSERT INTO capabilities (id, tools_json) VALUES (?, ?)
		ON CONFLICT(id) DO UPDATE SET tools_json = excluded.tools_json`, c.ID, string(toolsJSON))
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
