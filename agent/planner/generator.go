package planner

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	capgraph "github.com/OctoSucker/octosucker-utils/graph"
	"github.com/OctoSucker/octosucker/agent/llm"
)

const generateCapabilitySystemPrompt = `You generate the next capability step for an agent. Reply with exactly one JSON object, no other text.
Schema:
{
  "name": "snake_case_name",
  "description": "one line for LLM to choose this capability",
  "tool": "must be one of the available tools listed",
  "parameters": { "type": "object", "properties": { ... }, "required": [...] }
}
Rules:
- "tool" MUST be exactly one of the available tool names.
- "name" must be unique, snake_case.
- "parameters" must be valid JSON Schema for that tool.`

type Generator struct {
	llm *llm.LLMClient
}

func NewGenerator(llmClient *llm.LLMClient) *Generator {
	return &Generator{llm: llmClient}
}

type generatedCapability struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Tool        string                 `json:"tool"`
	Parameters  map[string]interface{} `json:"parameters"`
}

func (g *Generator) Generate(ctx context.Context, query string, existingCapNames []string, availableToolNames []string) (*capgraph.Node, error) {
	if len(availableToolNames) == 0 {
		return nil, fmt.Errorf("no available tools for dynamic capability")
	}
	toolsList := strings.Join(availableToolNames, ", ")
	existingList := strings.Join(existingCapNames, ", ")
	if existingList == "" {
		existingList = "(none yet)"
	}
	userPrompt := fmt.Sprintf(`User goal: %s

Existing capabilities (do not duplicate names): %s

Available tools (you MUST pick one): %s

Generate the next capability needed. Reply with only the JSON object.`, query, existingList, toolsList)

	content, err := g.llm.ChatCompletionWithSystem(ctx, generateCapabilitySystemPrompt, userPrompt)
	if err != nil {
		return nil, err
	}
	content = strings.TrimSpace(content)
	content = trimJSONBlock(content)
	var gen generatedCapability
	if err := json.Unmarshal([]byte(content), &gen); err != nil {
		log.Printf("[generator] invalid JSON: %s", content)
		return nil, fmt.Errorf("parse generated capability: %w", err)
	}
	gen.Tool = strings.TrimSpace(gen.Tool)
	gen.Name = strings.TrimSpace(gen.Name)
	if gen.Name == "" {
		gen.Name = "dynamic_step"
	}
	validTool := false
	for _, t := range availableToolNames {
		if t == gen.Tool {
			validTool = true
			break
		}
	}
	if !validTool {
		return nil, fmt.Errorf("generated tool %q not in available tools", gen.Tool)
	}
	for _, n := range existingCapNames {
		if n == gen.Name {
			gen.Name = gen.Name + "_" + gen.Tool
			break
		}
	}
	params := gen.Parameters
	if params == nil {
		params = map[string]interface{}{"type": "object", "properties": map[string]interface{}{}}
	}
	return &capgraph.Node{
		Name:        gen.Name,
		Description: gen.Description,
		InputSchema: params,
		Tool:        gen.Tool,
		Next:        []string{"finish"},
		Dynamic:     true,
	}, nil
}

func trimJSONBlock(s string) string {
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	return strings.TrimSpace(s)
}
