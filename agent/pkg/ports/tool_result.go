package ports

import (
	"encoding/json"
	"errors"
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

type Observation struct {
	Summary    string
	Structured any
	Err        error
}

func (o Observation) isZero() bool {
	return o.Summary == "" && o.Structured == nil && o.Err == nil
}

// MarshalJSON persists Summary, Structured, and Err as a string field "error".
func (o Observation) MarshalJSON() ([]byte, error) {
	type enc struct {
		Summary    string `json:"summary,omitempty"`
		Structured any    `json:"structured,omitempty"`
		ErrMsg     string `json:"error,omitempty"`
	}
	e := enc{Summary: o.Summary, Structured: o.Structured}
	if o.Err != nil {
		e.ErrMsg = o.Err.Error()
	}
	return json.Marshal(e)
}

// UnmarshalJSON restores Err from optional "error" string.
func (o *Observation) UnmarshalJSON(b []byte) error {
	type dec struct {
		Summary    string `json:"summary,omitempty"`
		Structured any    `json:"structured,omitempty"`
		ErrMsg     string `json:"error,omitempty"`
	}
	var d dec
	if err := json.Unmarshal(b, &d); err != nil {
		return err
	}
	o.Summary = d.Summary
	o.Structured = d.Structured
	if d.ErrMsg != "" {
		o.Err = errors.New(d.ErrMsg)
	} else {
		o.Err = nil
	}
	return nil
}
