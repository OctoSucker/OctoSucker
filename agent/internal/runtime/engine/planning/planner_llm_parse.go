package planning

import (
	"context"
	"fmt"
	"log"
	"maps"
	"slices"

	"github.com/OctoSucker/agent/pkg/ports"
)

type llmPlanResponse struct {
	Steps []struct {
		ID         string         `json:"id"`
		Goal       string         `json:"goal"`
		Capability string         `json:"capability"`
		Tool       string         `json:"tool"`
		DependsOn  []string       `json:"depends_on"`
		Arguments  map[string]any `json:"arguments"`
	} `json:"steps"`
}

func (p *Planner) completeAndParseLLMPlan(ctx context.Context, taskID string, system string, user string) (*ports.Plan, error) {
	var x llmPlanResponse
	if err := p.PlannerLLM.CompleteJSON(ctx, system, user, &x); err != nil || len(x.Steps) == 0 {
		log.Printf("engine.Dispatcher: invalid plan JSON task=%s err=%v", taskID, err)
		return nil, fmt.Errorf("planner: llm returned invalid or empty plan json")
	}
	if p.CapRegistry == nil {
		return nil, fmt.Errorf("planner: nil CapRegistry")
	}
	validCaps := p.CapRegistry.AllCapabilities()
	parsed := &ports.Plan{}
	for _, st := range x.Steps {
		if st.ID == "" || st.Capability == "" {
			log.Printf("engine.Dispatcher: invalid plan JSON task=%s", taskID)
			return nil, fmt.Errorf("planner: llm returned invalid or empty plan json")
		}
		capEntry, ok := validCaps[st.Capability]
		if !ok {
			log.Printf("engine.Dispatcher: invalid plan JSON task=%s", taskID)
			return nil, fmt.Errorf("planner: llm returned invalid or empty plan json")
		}
		if st.Tool != "" && !slices.Contains(capEntry.Tools, st.Tool) {
			log.Printf("engine.Dispatcher: invalid plan JSON task=%s tool=%q cap=%q", taskID, st.Tool, st.Capability)
			return nil, fmt.Errorf("planner: llm returned invalid or empty plan json")
		}
		parsed.Steps = append(parsed.Steps, ports.PlanStep{
			ID:         st.ID,
			Goal:       st.Goal,
			Capability: st.Capability,
			Tool:       st.Tool,
			DependsOn:  st.DependsOn,
			Arguments:  maps.Clone(st.Arguments),
			Status:     "pending",
		})
	}
	return parsed, nil
}
