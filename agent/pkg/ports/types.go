package ports

import (
	"fmt"
)

type ToolCall struct {
	Name      string
	Arguments map[string]any
}

type ToolResult struct {
	OK     bool
	Output any
	Err    error
}

func (res ToolResult) Observation() Observation {
	if res.Err != nil {
		return Observation{Summary: res.Err.Error(), Err: res.Err}
	}
	if !res.OK {
		return Observation{Summary: "tool failed", Err: fmt.Errorf("not ok")}
	}
	s := fmt.Sprint(res.Output)
	if len(s) > 500 {
		s = s[:500] + "…"
	}
	return Observation{Summary: s, Structured: res.Output}
}

type Capability struct {
	ID    string   `json:"id"`
	Tools []string `json:"tools"`
}

type CapabilityInvocation struct {
	CapabilityID string
	Tool         string
	Arguments    map[string]any
}

type RouteMode string

const (
	RouteSkill   RouteMode = "skill"
	RouteGraph   RouteMode = "graph"
	RoutePlanner RouteMode = "planner"
)

type RoutingContext struct {
	TaskType   string
	IntentText string
	Embedding  []float32
	Cost       float64
	Latency    float64
}

type Observation struct {
	Summary    string
	Structured any
	Err        error
}
