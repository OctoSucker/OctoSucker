package planning

import (
	"context"
	"fmt"
	"log"
	"maps"

	"github.com/OctoSucker/agent/pkg/ports"
)

type llmPlanResponse struct {
	Steps []struct {
		ID         string         `json:"id"`
		Goal       string         `json:"goal"`
		Capability string         `json:"capability"`
		DependsOn  []string       `json:"depends_on"`
		Arguments  map[string]any `json:"arguments"`
	} `json:"steps"`
}

func (p *Planner) completeAndParseLLMPlan(ctx context.Context, taskID string, system string, user string) (*ports.Plan, error) {
	var x llmPlanResponse
	if err := p.PlannerLLM.CompleteJSON(ctx, system, user, &x); err != nil || len(x.Steps) == 0 {
		log.Printf("engine.Dispatcher: invalid plan JSON session=%s err=%v", taskID, err)
		return nil, fmt.Errorf("planner: llm returned invalid or empty plan json")
	}
	parsed := &ports.Plan{}
	for _, st := range x.Steps {
		if st.ID == "" || st.Capability == "" {
			log.Printf("engine.Dispatcher: invalid plan JSON session=%s", taskID)
			return nil, fmt.Errorf("planner: llm returned invalid or empty plan json")
		}
		if _, ok := p.ValidPlanCapabilities[st.Capability]; !ok {
			log.Printf("engine.Dispatcher: invalid plan JSON session=%s", taskID)
			return nil, fmt.Errorf("planner: llm returned invalid or empty plan json")
		}
		parsed.Steps = append(parsed.Steps, ports.PlanStep{
			ID:         st.ID,
			Goal:       st.Goal,
			Capability: st.Capability,
			DependsOn:  st.DependsOn,
			Arguments:  maps.Clone(st.Arguments),
			Status:     "pending",
		})
	}
	return parsed, nil
}
