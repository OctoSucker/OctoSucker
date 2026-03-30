package graph

// Key identifies a directed edge in the routing graph.
type Key struct {
	From Node
	To   Node
}

// EdgeStat is per-edge empirical stats: endpoints plus success/failure mass and optional cost/latency (scoring, persistence).
// In-memory map entries may omit From/To when redundant with Key; SQLite rows fill From/To from Key.
type EdgeStat struct {
	From    Node    `json:"from"`
	To      Node    `json:"to"`
	Success float64 `json:"success"`
	Failure float64 `json:"failure"`
	Cost    float64 `json:"cost,omitempty"`
	Latency float64 `json:"latency,omitempty"`
}

// ContextTransition is one recent (intent, from→to) observation for intent-similarity scoring (JSON in meta).
type ContextTransition struct {
	Intent  string `json:"intent"`
	From    string `json:"from"`
	To      string `json:"to"`
	Outcome bool   `json:"outcome"` // true: success, false: failure
}
