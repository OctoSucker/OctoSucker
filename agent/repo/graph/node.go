package graph

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Node is a routing vertex: a single globally unique tool name.
type Node struct {
	Tool string `json:"tool"`
}

func trimTool(s string) string {
	return strings.TrimSpace(s)
}

// IsEntry reports whether n is the synthetic entry vertex.
func (n Node) IsEntry() bool {
	return trimTool(n.Tool) == ""
}

// IsValid reports a real routing vertex (non-empty tool name, no "::" which is reserved as invalid).
func (n Node) IsValid() bool {
	t := trimTool(n.Tool)
	return t != "" && !strings.Contains(t, "::")
}

// MakeNode builds a vertex from a tool name.
func MakeNode(tool string) *Node {
	t := trimTool(tool)
	if t == "" || strings.Contains(t, "::") {
		return &Node{}
	}
	return &Node{Tool: t}
}

// String returns the canonical id, or "" for the entry vertex.
func (n Node) String() string {
	if n.IsEntry() {
		return ""
	}
	return trimTool(n.Tool)
}

// ParseNode parses a canonical id. The empty string is the entry vertex (ok true).
func ParseNode(s string) (Node, bool) {
	if s == "" {
		return Node{}, true
	}
	t := trimTool(s)
	if t == "" || strings.Contains(t, "::") {
		return Node{}, false
	}
	return Node{Tool: t}, true
}

// MarshalJSON emits only {"tool": "..."} for planner and persisted plans.
func (n Node) MarshalJSON() ([]byte, error) {
	type out struct {
		Tool string `json:"tool"`
	}
	return json.Marshal(out{Tool: n.Tool})
}

// UnmarshalJSON accepts {"tool":"name"} only.
func (n *Node) UnmarshalJSON(b []byte) error {
	type raw struct {
		Tool string `json:"tool"`
	}
	var R raw
	if err := json.Unmarshal(b, &R); err != nil {
		return err
	}
	n.Tool = trimTool(R.Tool)
	return nil
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
		node, ok := ParseNode(x)
		if !ok || !node.IsValid() {
			return fmt.Errorf("graph: invalid path node %q", x)
		}
		out = append(out, node)
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
