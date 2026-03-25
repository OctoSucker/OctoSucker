package planning

func (p *Planner) buildPlannerSystemPrompt() string {
	system := p.PlanSystemPrompt + "\n\n" + p.ToolAppendix
	if hint := p.NodeFailures.HintForCapabilities(p.ValidPlanCapabilities); hint != "" {
		system += "\n\n" + hint
	}
	system += "\n\nEach step may include optional \"arguments\": a JSON object used as MCP tools/call arguments for that step. Only keys listed under that tool's params JSON Schema may appear; do not copy the user's message into \"arguments\" unless the schema has a matching field (e.g. send_telegram_message.text). Tools whose schema has no properties must use {} or omit \"arguments\". If one capability runs multiple tools in sequence, the same arguments object is sent to each—use separate steps when schemas differ."
	return system
}
