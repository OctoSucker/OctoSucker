package planning

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"strings"

	"github.com/OctoSucker/agent/pkg/ports"
	"github.com/OctoSucker/agent/repo/graph"
	"github.com/google/uuid"
)

// buildGraphPlan uses routing-graph frontier to choose a concrete capability/tool, then asks LLM to fill only that tool's arguments.
func (p *Planner) buildGraphPlan(ctx context.Context, taskID string, task *ports.Task, excludeCapability, excludeTool string) (*ports.Plan, error) {
	intent := task.UserInput.Text
	snap := task.RouteSnap
	frontier, err := p.RouteGraph.Frontier(ctx, intent, snap.LastNode, snap.LastOut)
	if err != nil {
		return nil, fmt.Errorf("planner: graph frontier: %w", err)
	}
	if len(frontier) == 0 {
		return nil, fmt.Errorf("planner: graph frontier is empty for task %s", taskID)
	}
	candidateNodes := make([]graph.Node, 0, len(frontier))
	for _, node := range frontier {
		if excludeCapability != "" && node.Capability == excludeCapability {
			continue
		}
		if excludeTool != "" && node.Tool == excludeTool {
			continue
		}
		candidateNodes = append(candidateNodes, node)
	}
	if len(candidateNodes) == 0 {
		return nil, fmt.Errorf("planner: no runnable node in graph frontier for task %s", taskID)
	}
	selectedNode := candidateNodes[0]
	onImmediate := p.RouteGraph.FilterCandidatesOnImmediateEdge(snap.LastNode, candidateNodes)
	if len(onImmediate) > 0 {
		if bestNode, ok := p.RouteGraph.PickBestByImmediateEdge(ctx, intent, snap.LastNode, onImmediate); ok {
			selectedNode = bestNode
		}
	}
	if !selectedNode.IsValid() {
		return nil, fmt.Errorf("planner: selected graph node is invalid")
	}
	capID, tool := selectedNode.Capability, selectedNode.Tool

	toolSpec, err := p.RouteGraph.Tool(capID, tool)
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
		task.UserInput.Text, selectedNode.String(), capID, tool, string(schemaRaw),
	)
	if rh := strings.TrimSpace(task.PlannerReplanHint); rh != "" {
		argUser = "## Recent tool failure (automatic replan)\n" + rh + "\n\n" + argUser
	}
	if err := p.PlannerLLM.CompleteJSON(ctx, argSystem, argUser, &args); err != nil {
		return nil, fmt.Errorf("planner: graph plan arguments json: %w", err)
	}
	if args == nil {
		args = map[string]any{}
	}

	plan := &ports.Plan{
		Steps: []*ports.PlanStep{{
			ID:         uuid.New().String(),
			Goal:       task.UserInput.Text,
			Capability: capID,
			Tool:       tool,
			Arguments:  maps.Clone(args),
			Status:     "pending",
		}},
	}
	return plan, nil
}
