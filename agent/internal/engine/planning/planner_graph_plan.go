package planning

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"

	routinggraph "github.com/OctoSucker/agent/internal/store/routing_graph"
	"github.com/OctoSucker/agent/pkg/ports"
)

// buildGraphPlan uses routing-graph frontier to choose a concrete capability/tool, then asks LLM to fill only that tool's arguments.
func (p *Planner) buildGraphPlan(ctx context.Context, taskID string, taskState *ports.Task, excludeCapability, excludeTool string) (*ports.Plan, error) {
	rc := ports.RoutingContext{IntentText: taskState.UserInput.Text}
	snap := taskState.RouteSnap
	frontier, err := p.RouteGraph.Frontier(ctx, rc, snap.LastNode, snap.LastOut)
	if err != nil {
		return nil, fmt.Errorf("planner: graph frontier: %w", err)
	}
	if len(frontier) == 0 {
		return nil, fmt.Errorf("planner: graph frontier is empty for task %s", taskID)
	}
	candidateNodes := make([]string, 0, len(frontier))
	for _, nodeID := range frontier {
		c, t, ok := routinggraph.ParseNodeID(nodeID)
		if !ok {
			continue
		}
		if excludeCapability != "" && c == excludeCapability {
			continue
		}
		if excludeTool != "" && t == excludeTool {
			continue
		}
		candidateNodes = append(candidateNodes, nodeID)
	}
	if len(candidateNodes) == 0 {
		return nil, fmt.Errorf("planner: no runnable node in graph frontier for task %s", taskID)
	}
	selectedNode := candidateNodes[0]
	onImmediate := p.RouteGraph.FilterCandidatesOnImmediateEdge(snap.LastNode, candidateNodes)
	if len(onImmediate) > 0 {
		if bestNode, ok := p.RouteGraph.PickBestByImmediateEdge(ctx, rc, snap.LastNode, onImmediate); ok {
			selectedNode = bestNode
		}
	}
	capID, tool, ok := routinggraph.ParseNodeID(selectedNode)
	if !ok {
		return nil, fmt.Errorf("planner: selected graph node %q is invalid", selectedNode)
	}

	toolSpec, err := p.CapRegistry.Tool(ctx, capID, tool)
	if err != nil {
		return nil, fmt.Errorf("planner: graph plan tool spec: %w", err)
	}
	schemaRaw, err := json.Marshal(toolSpec.InputSchema)
	if err != nil {
		return nil, fmt.Errorf("planner: marshal tool input schema: %w", err)
	}

	var args map[string]any
	argSystem := "You generate ONLY a JSON object for tool arguments. Return one JSON object, no markdown, no extra text."
	argUser := fmt.Sprintf(
		"Task: %s\nSelected node: %s\nCapability: %s\nTool: %s\nTool input schema JSON:\n%s\n\nReturn ONLY arguments JSON object for this tool.",
		taskState.UserInput.Text, selectedNode, capID, tool, string(schemaRaw),
	)
	if err := p.PlannerLLM.CompleteJSON(ctx, argSystem, argUser, &args); err != nil {
		return nil, fmt.Errorf("planner: graph plan arguments json: %w", err)
	}
	if args == nil {
		args = map[string]any{}
	}

	plan := &ports.Plan{
		Steps: []ports.PlanStep{{
			ID:         "graph-step",
			Goal:       taskState.UserInput.Text,
			Capability: capID,
			Tool:       tool,
			DependsOn:  nil,
			Arguments:  maps.Clone(args),
			Status:     "pending",
		}},
	}
	return reassignPlanStepIDsWithUUID(plan)
}
