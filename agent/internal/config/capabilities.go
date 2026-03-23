package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/OctoSucker/agent/pkg/ports"
)

func LoadCapabilities(path string) (map[string]ports.Capability, error) {
	if path == "" {
		return nil, fmt.Errorf("capabilities: path is required")
	}
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	p := path
	if st.IsDir() {
		p = filepath.Join(path, "default_agent_capabilities.json")
	}
	raw, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	var doc struct {
		Capabilities map[string]ports.Capability `json:"capabilities"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	m := doc.Capabilities
	if len(m) == 0 {
		return nil, fmt.Errorf("capabilities: capabilities required")
	}
	for id, c := range m {
		if id == "" {
			return nil, fmt.Errorf("capabilities: empty capability key")
		}
		if c.ID != "" && c.ID != id {
			return nil, fmt.Errorf("capabilities: key %q must match id %q", id, c.ID)
		}
		if c.ID == "" {
			c.ID = id
			m[id] = c
		}
		if len(c.Tools) == 0 {
			return nil, fmt.Errorf("capabilities: capability %q has no tools", id)
		}
	}
	return m, nil
}
