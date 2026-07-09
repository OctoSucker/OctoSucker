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
- the selected step goal
- optional context from prior executed steps
- accumulated evidence, including prior failed actions
- a specific tool name (flat id)
- a JSON schema describing the tool input

You must generate arguments that strictly follow the schema.
The selected step goal is the immediate action to parameterize; use it to avoid repeating a failed command from the broader user task.
If [ACCUMULATED EVIDENCE] contains a failed action with concrete arguments, do not repeat the same tool arguments unless the selected step goal explicitly explains a meaningful change.

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
9. For tool "read_skill", argument "name" must be one exact value from [AVAILABLE SKILLS].
10. Website names, app names, provider ids, and operation targets are not skill names unless they appear in [AVAILABLE SKILLS].

--------------------------------------------------
SELF CHECK BEFORE OUTPUT

- Is JSON valid?
- Does it match the schema?
- Are all required fields present?
- Are types correct?

Then output JSON only.
`

// buildToolArgumentsPromptPair returns system + user messages for a single-tool argument JSON completion.
// priorRunsContext is optional; when non-empty it is included so the model can copy values from earlier steps.
func (p *Planner) buildToolArgumentsPromptPair(userGoal, stepGoal, toolID, priorRunsContext, evidence string) (system string, user string, err error) {
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
	if s := strings.TrimSpace(stepGoal); s != "" {
		fmt.Fprintf(&b, "\n[SELECTED STEP GOAL]\n%s\n", s)
	}
	if s := strings.TrimSpace(p.ProjectCtx); s != "" {
		fmt.Fprintf(&b, "\n[PROJECT INSTRUCTIONS]\n%s\n", s)
	}
	if s := strings.TrimSpace(priorRunsContext); s != "" {
		fmt.Fprintf(&b, "\n[CONTEXT FROM PRIOR RUNS — use for argument values when relevant]\n%s\n", s)
	}
	if s := strings.TrimSpace(evidence); s != "" {
		fmt.Fprintf(&b, "\n[ACCUMULATED EVIDENCE — includes prior failures]\n%s\n", s)
	}
	if toolID == "read_skill" {
		fmt.Fprintf(&b, "\n[AVAILABLE SKILLS]\n%s\n", plannerJSON(p.skillNames()))
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
