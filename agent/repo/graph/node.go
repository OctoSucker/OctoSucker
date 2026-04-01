package graph

import (
	"encoding/json"
	"fmt"
	"strings"
)

const routingNodeSep = "::"

type Node struct {
	Capability string `json:"capability"`
	Tool       string `json:"tool"`
}

// IsEntry reports whether n is the synthetic entry vertex.
func (n Node) IsEntry() bool {
	return n.Capability == "" && n.Tool == ""
}

// IsValid reports a real routing vertex (non-empty capability and tool).
func (n Node) IsValid() bool {
	return n.Capability != "" && n.Tool != ""
}

// MakeNode builds a vertex from capability and tool strings.
func MakeNode(capability, tool string) *Node {
	return &Node{Capability: capability, Tool: tool}
}

// String returns the canonical id (cap::tool), or "" for the entry vertex.
func (n Node) String() string {
	if n.IsEntry() {
		return ""
	}
	return n.Capability + routingNodeSep + n.Tool
}

// ParseNode parses a canonical id. The empty string is the entry vertex (ok true).
func ParseNode(s string) (Node, bool) {
	if s == "" {
		return Node{}, true
	}
	parts := strings.SplitN(s, routingNodeSep, 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return Node{}, false
	}
	return Node{Capability: parts[0], Tool: parts[1]}, true
}

// RoutePath is a sequence of routing vertices. JSON is a string array of canonical ids.
type RoutePath []Node

func (p RoutePath) MarshalJSON() ([]byte, error) {
	if p == nil {
		return []byte("null"), nil
	}
	s := make([]string, len(p))
	for i, n := range p {
		if !n.IsValid() {
			return nil, fmt.Errorf("graph: RoutePath: invalid node at index %d", i)
		}
		s[i] = n.String()
	}
	return json.Marshal(s)
}

func (p *RoutePath) UnmarshalJSON(b []byte) error {
	var raw []string
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	out := make([]Node, 0, len(raw))
	for _, x := range raw {
		n, ok := ParseNode(x)
		if !ok || !n.IsValid() {
			return fmt.Errorf("graph: invalid path node %q", x)
		}
		out = append(out, n)
	}
	*p = out
	return nil
}

// RoutePathsEqual compares two paths by value.
func RoutePathsEqual(a, b RoutePath) bool {
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
