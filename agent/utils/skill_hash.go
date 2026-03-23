package utils

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"maps"
	"sort"
	"strings"

	"github.com/OctoSucker/agent/pkg/ports"
)

// HashPipeJoinedCapabilities returns first 8 hex chars of SHA256(caps joined by "|").
func HashPipeJoinedCapabilities(caps []string) string {
	h := sha256.Sum256([]byte(strings.Join(caps, "|")))
	return hex.EncodeToString(h[:])[:8]
}

// PlanSemanticFingerprint is a stable hash over plan semantics (steps + deps + arguments); 12 hex chars.
func PlanSemanticFingerprint(p *ports.Plan) string {
	if p == nil || len(p.Steps) == 0 {
		return ""
	}
	type sigStep struct {
		ID         string         `json:"id"`
		Capability string         `json:"capability"`
		Goal       string         `json:"goal"`
		DependsOn  []string       `json:"depends_on"`
		Arguments  map[string]any `json:"arguments,omitempty"`
	}
	steps := make([]sigStep, len(p.Steps))
	for i, st := range p.Steps {
		dep := append([]string(nil), st.DependsOn...)
		sort.Strings(dep)
		steps[i] = sigStep{
			ID: st.ID, Capability: st.Capability, Goal: st.Goal,
			DependsOn: dep, Arguments: maps.Clone(st.Arguments),
		}
	}
	b, err := json.Marshal(steps)
	if err != nil {
		return ""
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])[:12]
}

// StringSlicesEqual compares two string slices element-wise.
func StringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
