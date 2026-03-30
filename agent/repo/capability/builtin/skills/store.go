package skillsbuiltin

import (
	"bytes"
	"encoding/json"
	"fmt"
)

type SkillTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Usage       string         `json:"usage"`
	InputSchema map[string]any `json:"input_schema,omitempty"`
}

type SkillCapability struct {
	Capability string      `json:"capability"`
	Tools      []SkillTool `json:"tools"`
}

type Skill struct {
	Name         string            `json:"name"`
	Description  string            `json:"description"`
	Cautions     string            `json:"cautions,omitempty"`
	SourcePath   string            `json:"source_path"`
	Capabilities []SkillCapability `json:"capabilities"`
}

func decodeSkillJSON(data []byte) (Skill, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var sk Skill
	if err := dec.Decode(&sk); err != nil {
		return Skill{}, err
	}
	if dec.More() {
		return Skill{}, fmt.Errorf("skills builtin: trailing JSON after skill")
	}
	return sk, nil
}
