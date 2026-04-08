package thinker

import (
	"context"
	"fmt"
	"strings"

	"github.com/OctoSucker/octosucker/engine/types"
	"github.com/OctoSucker/octosucker/pkg/llmclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	// ToolExtractEntityCorrelations reads arbitrary text and returns every stated entity–entity correlation.
	ToolExtractEntityCorrelations = "think_extract_entity_correlations"
	// ToolParseSingleEntityCorrelation parses one short sentence or clause into one directed correlation tuple.
	ToolParseSingleEntityCorrelation = "think_parse_single_entity_correlation"
)

type Runner struct {
	llm *llmclient.OpenAI
}

func NewRunner(llm *llmclient.OpenAI) (*Runner, error) {
	if llm == nil {
		return nil, fmt.Errorf("thinker builtin: llm client is required")
	}
	return &Runner{llm: llm}, nil
}

func (r *Runner) Name() (string, string) {
	return "thinker", "LLM extraction of entity pairs and positive/negative correlation from natural language."
}

func (r *Runner) HasTool(name string) bool {
	switch strings.TrimSpace(name) {
	case ToolExtractEntityCorrelations, ToolParseSingleEntityCorrelation:
		return true
	default:
		return false
	}
}

func (r *Runner) Tool(tool string) (*mcp.Tool, error) {
	switch strings.TrimSpace(tool) {
	case ToolExtractEntityCorrelations:
		return &mcp.Tool{
			Name: ToolExtractEntityCorrelations,
			Description: "From free-form text, extract directed entity relationships with correlation sign. " +
				"Use short canonical entity ids for underlying domains (e.g. 信贷, 经济), not duplicate parallel phrases (e.g. 信贷扩张→经济繁荣 and 信贷收缩→经济下行) when they describe one same-domain association. " +
				"positive=true: same-direction association; positive=false: negative/opposite. " +
				"Output relations as {from_id, to_id, positive}. Use kg_add_edge; endpoints are created if missing.",
			InputSchema: schemaText("text", "Passage that may state several entity correlations."),
		}, nil
	case ToolParseSingleEntityCorrelation:
		return &mcp.Tool{
			Name: ToolParseSingleEntityCorrelation,
			Description: "Parse one sentence or short phrase into a single directed tuple {from_id, to_id, positive} (same semantics as think_extract_entity_correlations). " +
				"If the phrase does not express one clear pair, the model should still pick the single best reading.",
			InputSchema: schemaText("text", "One sentence or short phrase describing how two entities relate."),
		}, nil
	default:
		return nil, fmt.Errorf("thinker builtin: unknown tool %q", tool)
	}
}

func (r *Runner) ToolList(ctx context.Context) ([]*mcp.Tool, error) {
	names := []string{ToolExtractEntityCorrelations, ToolParseSingleEntityCorrelation}
	out := make([]*mcp.Tool, 0, len(names))
	for _, n := range names {
		t, err := r.Tool(n)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, nil
}

type relation struct {
	FromID   string `json:"from_id"`
	ToID     string `json:"to_id"`
	Positive bool   `json:"positive"`
}

type multiExtract struct {
	Relations []relation `json:"relations"`
}

const multiSystem = `You extract directed entity pairs and correlation polarity from the user's text.

Return ONLY valid JSON (no markdown fences). Shape:
{"relations":[{"from_id":"<concise entity>","to_id":"<concise entity>","positive":true|false},...]}

Entity naming (critical):
- Use short canonical ids for the underlying domains or actors (e.g. 信贷, 经济, 市场), not long situational compounds when they are two sides of the same mechanism.
- Parallel contrast that illustrates ONE association: if the text says both "when X expands, Y booms" and "when X contracts, Y slumps" (same X-domain and Y-domain), that is one positive same-direction link between the core concepts behind X and Y — emit a single relation, e.g. from_id "信贷" to_id "经济", positive true — NOT two edges like 信贷扩张→经济繁荣 and 信贷收缩→经济下行.
- Only emit multiple relations when the text truly asserts different pairwise mechanisms (different core entity pairs or different causal stories).

Direction and polarity:
- Use the same language as the source for entity names when the text is not English.
- from_id → to_id follows the stated or implied driver/target in the prose (e.g. "A导致B下跌" → from_id A, to_id B, positive false).
- positive=true: same-direction association (co-movement the same way).
- positive=false: negative correlation or opposite movement.

Coverage:
- Include every distinct core relation implied; omit anything not supported by the text.
- If there are no such relations, return {"relations":[]}.`

const singleSystem = `You parse ONE stated relationship between two entities from the input.

Return ONLY valid JSON (no markdown fences). Shape:
{"from_id":"<concise entity>","to_id":"<concise entity>","positive":true|false}

Same rules as batch extraction: prefer short canonical entity names for underlying concepts (not redundant compound phrases). Directed from_id → to_id, positive=true same-direction, positive=false opposite/negative correlation. Use the source language for entity names.`

func (r *Runner) Invoke(ctx context.Context, localTool string, arguments map[string]any) (types.ToolResult, error) {
	if r == nil || r.llm == nil {
		return types.ToolResult{Err: fmt.Errorf("thinker builtin: not initialized")}, fmt.Errorf("thinker builtin: not initialized")
	}
	switch localTool {
	case ToolExtractEntityCorrelations:
		text, err := parseRequiredText(arguments, ToolExtractEntityCorrelations)
		if err != nil {
			return types.ToolResult{Err: err}, err
		}
		var out multiExtract
		if err := r.llm.CompleteJSON(ctx, multiSystem, text, &out); err != nil {
			return types.ToolResult{Err: fmt.Errorf("thinker builtin: %s: %w", ToolExtractEntityCorrelations, err)},
				fmt.Errorf("thinker builtin: %s: %w", ToolExtractEntityCorrelations, err)
		}
		rows, err := validateRelations(out.Relations, ToolExtractEntityCorrelations)
		if err != nil {
			return types.ToolResult{Err: err}, err
		}
		return types.ToolResult{Output: map[string]any{"relations": rows}}, nil

	case ToolParseSingleEntityCorrelation:
		text, err := parseRequiredText(arguments, ToolParseSingleEntityCorrelation)
		if err != nil {
			return types.ToolResult{Err: err}, err
		}
		var one relation
		if err := r.llm.CompleteJSON(ctx, singleSystem, text, &one); err != nil {
			return types.ToolResult{Err: fmt.Errorf("thinker builtin: %s: %w", ToolParseSingleEntityCorrelation, err)},
				fmt.Errorf("thinker builtin: %s: %w", ToolParseSingleEntityCorrelation, err)
		}
		fromID := strings.TrimSpace(one.FromID)
		toID := strings.TrimSpace(one.ToID)
		if fromID == "" || toID == "" {
			err := fmt.Errorf("thinker builtin: %s: empty from_id/to_id in model output", ToolParseSingleEntityCorrelation)
			return types.ToolResult{Err: err}, err
		}
		return types.ToolResult{Output: map[string]any{
			"from_id":  fromID,
			"to_id":    toID,
			"positive": one.Positive,
		}}, nil

	default:
		err := fmt.Errorf("thinker builtin: unknown tool %q", localTool)
		return types.ToolResult{Err: err}, err
	}
}

func schemaText(field, fieldDesc string) map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			field: map[string]any{
				"type":        "string",
				"description": fieldDesc,
			},
		},
		"required":             []string{field},
		"additionalProperties": false,
	}
}

func parseRequiredText(args map[string]any, tool string) (string, error) {
	if args == nil {
		return "", fmt.Errorf("thinker builtin: %s: arguments required", tool)
	}
	raw, ok := args["text"]
	if !ok {
		return "", fmt.Errorf("thinker builtin: %s: text is required", tool)
	}
	s, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("thinker builtin: %s: text must be a string", tool)
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return "", fmt.Errorf("thinker builtin: %s: text must be non-empty", tool)
	}
	return s, nil
}

func validateRelations(rels []relation, tool string) ([]any, error) {
	out := make([]any, 0, len(rels))
	for i, rel := range rels {
		fromID := strings.TrimSpace(rel.FromID)
		toID := strings.TrimSpace(rel.ToID)
		if fromID == "" || toID == "" {
			return nil, fmt.Errorf("thinker builtin: %s: relation[%d] has empty from_id/to_id", tool, i)
		}
		out = append(out, map[string]any{
			"from_id":  fromID,
			"to_id":    toID,
			"positive": rel.Positive,
		})
	}
	return out, nil
}
