package routegraph

// Node is a routing vertex: a single globally unique tool name.
type Node struct {
	Tool string `json:"tool"`
}

// String returns the canonical id, or "" for the entry vertex.
func (n Node) String() string {
	return n.Tool
}
