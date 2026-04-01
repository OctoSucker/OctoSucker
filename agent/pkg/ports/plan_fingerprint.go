package ports

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"maps"
)

// PlanSemanticFingerprint is a stable hash over plan semantics (ordered steps + arguments); 12 hex chars.
func PlanSemanticFingerprint(p *Plan) string {
	if p == nil || len(p.Steps) == 0 {
		return ""
	}
	type sigStep struct {
		ID        string         `json:"id"`
		Node      string         `json:"node"`
		Goal      string         `json:"goal"`
		Arguments map[string]any `json:"arguments,omitempty"`
	}
	steps := make([]sigStep, len(p.Steps))
	for i, st := range p.Steps {
		steps[i] = sigStep{
			ID: st.ID, Node: st.Node.String(), Goal: st.Goal,
			Arguments: maps.Clone(st.Arguments),
		}
	}
	b, err := json.Marshal(steps)
	if err != nil {
		return ""
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])[:12]
}
