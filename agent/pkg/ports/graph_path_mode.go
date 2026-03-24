package ports

import "strings"

// GraphPathMode selects how capabilities are resolved along the routing graph during execution.
// It applies when RouteGraph / capability routing is used (not the pure LLM plan text alone).
type GraphPathMode string

const (
	// GraphPathGreedy: prefer next capability by local Frontier score (success × intent similarity × exploration; current default).
	GraphPathGreedy GraphPathMode = "greedy"
	// GraphPathGlobal: among feasible candidates, pick the one that minimizes w(last→c) + dist(c→finish) with learned edge weights (global-to-goal objective).
	GraphPathGlobal GraphPathMode = "global"
)

// ParseGraphPathMode maps config strings to GraphPathMode; unknown/empty → greedy.
func ParseGraphPathMode(s string) GraphPathMode {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "global", "global_opt", "global_optimal":
		return GraphPathGlobal
	default:
		return GraphPathGreedy
	}
}
