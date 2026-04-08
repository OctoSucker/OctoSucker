package planning

import (
	"encoding/json"
	"fmt"
	"strings"
)

const toolArgumentsGeneratorSystemPrompt = `
You are a tool argument generator for an AI agent.

Your job is to generate a valid JSON object for tool arguments.

You are given:
- a user task
- optional context from prior executed steps
- a specific tool name (flat id)
- a JSON schema describing the tool input

You must generate arguments that strictly follow the schema.

You are NOT chatting.
You are NOT planning.
You ONLY generate arguments.

--------------------------------------------------
STRICT RULES

1. Output MUST be a valid JSON object
2. Do NOT output anything except JSON
3. Do NOT include markdown
4. Do NOT include explanations
5. All required fields in schema MUST be present
6. Field types MUST match the schema
7. Do NOT invent fields not in schema — use only property names from [TOOL INPUT SCHEMA]
8. If no arguments are needed, return {}

--------------------------------------------------
SELF CHECK BEFORE OUTPUT

- Is JSON valid?
- Does it match the schema?
- Are all required fields present?
- Are types correct?

Then output JSON only.
`

// buildToolArgumentsPromptPair returns system + user messages for a single-tool argument JSON completion.
// priorRunsContext is optional; when non-empty it is included so the model can copy values (e.g. relations → kg_add_edge).
func (p *Planner) buildToolArgumentsPromptPair(userGoal, toolID, priorRunsContext string) (system string, user string, err error) {
	toolSpec, err := p.ToolRegistry.Tool(toolID)
	if err != nil {
		return "", "", fmt.Errorf("planner: tool arguments prompt tool %q: %w", toolID, err)
	}
	schemaRaw, err := json.Marshal(toolSpec.InputSchema)
	if err != nil {
		return "", "", fmt.Errorf("planner: marshal tool input schema: %w", err)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "[TASK]\n%s\n", userGoal)
	if s := strings.TrimSpace(priorRunsContext); s != "" {
		fmt.Fprintf(&b, "\n[CONTEXT FROM PRIOR RUNS — use for argument values when relevant]\n%s\n", s)
	}
	fmt.Fprintf(&b, `
[TOOL]
%s

[TOOL INPUT SCHEMA]
%s

Generate arguments for this tool.

Return ONLY a JSON object.
`, toolID, string(schemaRaw))
	return toolArgumentsGeneratorSystemPrompt, b.String(), nil
}
