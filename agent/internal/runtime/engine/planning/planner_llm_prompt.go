package planning

import "github.com/OctoSucker/agent/pkg/ports"

func (p *Planner) buildPlannerSystemPrompt() string {
	system := p.PlanSystemPrompt + "\n\n" + p.ToolAppendix
	var valid map[string]ports.Capability
	if p.CapRegistry != nil {
		valid = p.CapRegistry.AllCapabilities()
	}
	if hint := p.NodeFailures.HintForCapabilities(valid); hint != "" {
		system += "\n\n" + hint
	}
	system += "\n\nEach step may include optional \"tool\" (required when the chosen capability exposes multiple MCP tools): exact tool name from the appendix. Each step may include optional \"arguments\": only keys allowed by that tool's JSON Schema. If one capability runs multiple tools in sequence without a per-step \"tool\", the runtime uses the first tool only—prefer explicit \"tool\" per step when schemas differ."
	return system
}
