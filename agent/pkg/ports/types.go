package ports

import (
	"encoding/json"
	"fmt"
)

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
	var summary string
	if b, err := json.Marshal(res.Output); err == nil {
		summary = string(b)
		const maxSummaryRunes = 12000
		if len([]rune(summary)) > maxSummaryRunes {
			r := []rune(summary)
			summary = string(r[:maxSummaryRunes]) + "…"
		}
	} else {
		s := fmt.Sprint(res.Output)
		if len(s) > 500 {
			s = s[:500] + "…"
		}
		summary = s
	}
	return Observation{Summary: summary, Structured: res.Output}
}

type RouteMode string

const (
	RouteProcedure RouteMode = "procedure"
	RouteGraph     RouteMode = "graph"
	RoutePlanner   RouteMode = "planner"
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
