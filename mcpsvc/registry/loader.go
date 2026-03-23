package registry

import (
	"encoding/json"
	"fmt"
	"os"
)

func LoadFile(path string) ([]Plugin, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseJSON(b)
}

func ParseJSON(b []byte) ([]Plugin, error) {
	var list []Plugin
	if err := json.Unmarshal(b, &list); err != nil {
		return nil, fmt.Errorf("registry.ParseJSON: %w", err)
	}
	for _, p := range list {
		if p.ID == "" {
			return nil, fmt.Errorf("registry.ParseJSON: empty plugin id")
		}
	}
	return list, nil
}

func (r *Registry) LoadFile(path string) error {
	list, err := LoadFile(path)
	if err != nil {
		return err
	}
	for _, p := range list {
		r.Register(p)
	}
	return nil
}
