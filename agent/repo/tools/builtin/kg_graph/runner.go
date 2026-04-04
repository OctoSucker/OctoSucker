package kggraph

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/OctoSucker/agent/model"
	"github.com/OctoSucker/agent/pkg/llmclient"
	"github.com/OctoSucker/agent/pkg/ports"
	knowledgegraph "github.com/OctoSucker/agent/repo/knowledge_graph"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	ToolAddNode            = "kg_add_node"
	ToolAddEdge            = "kg_add_edge"
	ToolLookupNodeExact    = "kg_lookup_node_exact"
	ToolLookupNodeSemantic = "kg_lookup_node_semantic"
	ToolListNodes          = "kg_list_nodes"
	ToolListEdges          = "kg_list_edges"
)

// Runner runs knowledge-graph tools against workspace SQLite opened from workspaceRoot (implements tools.ToolProvider).
type Runner struct {
	db *model.AgentDB
	g  *knowledgegraph.Graph
}

// NewRunner opens <workspaceRoot>/data/octoplus.sqlite and builds the graph with llm for embeddings.
// The DB handle is held for the process lifetime (no Close hook).
func NewRunner(workspaceRoot string, llm *llmclient.OpenAI) (*Runner, error) {
	if llm == nil {
		return nil, fmt.Errorf("kg_graph builtin: llm is required")
	}
	wd := strings.TrimSpace(workspaceRoot)
	if wd == "" {
		return nil, fmt.Errorf("kg_graph builtin: workspace root is required")
	}
	abs, err := filepath.Abs(wd)
	if err != nil {
		return nil, fmt.Errorf("kg_graph builtin: resolve workspace: %w", err)
	}
	st, err := os.Stat(abs)
	if err != nil {
		return nil, fmt.Errorf("kg_graph builtin: workspace %q: %w", abs, err)
	}
	if !st.IsDir() {
		return nil, fmt.Errorf("kg_graph builtin: workspace %q is not a directory", abs)
	}
	db, err := model.OpenAgentDB(abs)
	if err != nil {
		return nil, fmt.Errorf("kg_graph builtin: open agent db: %w", err)
	}
	g, err := knowledgegraph.New(db, llm)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("kg_graph builtin: open graph: %w", err)
	}
	return &Runner{db: db, g: g}, nil
}

// Name is the ToolRegistry.Backends map key for this provider (not a user-facing tool id).
func (r *Runner) Name() string { return "kg_graph" }

func (r *Runner) HasTool(name string) bool {
	switch strings.TrimSpace(name) {
	case ToolAddNode, ToolAddEdge, ToolLookupNodeExact, ToolLookupNodeSemantic, ToolListNodes, ToolListEdges:
		return true
	default:
		return false
	}
}

func (r *Runner) Tool(tool string) (*mcp.Tool, error) {
	switch strings.TrimSpace(tool) {
	case ToolAddNode:
		return &mcp.Tool{
			Name:        ToolAddNode,
			Description: "Add a canonical knowledge-graph node id; stores an embedding for kg_lookup_node_semantic.",
			InputSchema: schemaAddNode(),
		}, nil
	case ToolAddEdge:
		return &mcp.Tool{
			Name:        ToolAddEdge,
			Description: "Add a directed influence edge between existing node ids. Use positive=false for negative correlation.",
			InputSchema: schemaAddEdge(),
		}, nil
	case ToolLookupNodeExact:
		return &mcp.Tool{
			Name:        ToolLookupNodeExact,
			Description: "Find a stored node id by exact string match on the term (no embedding API).",
			InputSchema: schemaTerm(),
		}, nil
	case ToolLookupNodeSemantic:
		return &mcp.Tool{
			Name:        ToolLookupNodeSemantic,
			Description: "Find a node id: try exact match on the term first, else cosine similarity against stored node embeddings (calls embedding API).",
			InputSchema: schemaTerm(),
		}, nil
	case ToolListNodes:
		return &mcp.Tool{
			Name:        ToolListNodes,
			Description: "List all knowledge-graph node ids in the workspace database.",
			InputSchema: schemaEmpty(),
		}, nil
	case ToolListEdges:
		return &mcp.Tool{
			Name:        ToolListEdges,
			Description: "List all directed edges (from_id, to_id, positive correlation flag).",
			InputSchema: schemaEmpty(),
		}, nil
	default:
		return nil, fmt.Errorf("kg_graph builtin: unknown tool %q", tool)
	}
}

func (r *Runner) ToolList(ctx context.Context) ([]*mcp.Tool, error) {
	names := []string{ToolAddNode, ToolAddEdge, ToolLookupNodeExact, ToolLookupNodeSemantic, ToolListNodes, ToolListEdges}
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

func (r *Runner) Invoke(ctx context.Context, localTool string, arguments map[string]any) (ports.ToolResult, error) {
	if r == nil || r.db == nil || r.g == nil {
		return ports.ToolResult{Err: fmt.Errorf("kg_graph builtin: not initialized")}, fmt.Errorf("kg_graph builtin: not initialized")
	}
	switch localTool {
	case ToolAddNode:
		id, err := parseRequiredString(arguments, ToolAddNode, "id")
		if err != nil {
			return ports.ToolResult{Err: err}, err
		}
		if err := r.g.AddNode(ctx, id); err != nil {
			return ports.ToolResult{Err: err}, err
		}
		return ports.ToolResult{Output: map[string]any{"id": id, "added": true}}, nil

	case ToolAddEdge:
		fromID, err := parseRequiredString(arguments, ToolAddEdge, "from_id")
		if err != nil {
			return ports.ToolResult{Err: err}, err
		}
		toID, err := parseRequiredString(arguments, ToolAddEdge, "to_id")
		if err != nil {
			return ports.ToolResult{Err: err}, err
		}
		positive, err := parseOptionalPositive(arguments)
		if err != nil {
			return ports.ToolResult{Err: err}, err
		}
		if err := r.g.AddEdge(fromID, toID, positive); err != nil {
			return ports.ToolResult{Err: err}, err
		}
		return ports.ToolResult{Output: map[string]any{"from_id": fromID, "to_id": toID, "positive": positive}}, nil

	case ToolLookupNodeExact:
		term, err := parseRequiredString(arguments, ToolLookupNodeExact, "term")
		if err != nil {
			return ports.ToolResult{Err: err}, err
		}
		canon, ok, err := r.g.CanonicalFor(term)
		if err != nil {
			return ports.ToolResult{Err: err}, err
		}
		if !ok {
			err := fmt.Errorf("kg_graph builtin: %s: no exact match for %q", ToolLookupNodeExact, term)
			return ports.ToolResult{Err: err}, err
		}
		return ports.ToolResult{Output: map[string]any{"canonical": canon, "matched": true}}, nil

	case ToolLookupNodeSemantic:
		term, err := parseRequiredString(arguments, ToolLookupNodeSemantic, "term")
		if err != nil {
			return ports.ToolResult{Err: err}, err
		}
		canon, ok, err := r.g.CanonicalForContext(ctx, term)
		if err != nil {
			return ports.ToolResult{Err: err}, err
		}
		if !ok {
			err := fmt.Errorf("kg_graph builtin: %s: no match for %q", ToolLookupNodeSemantic, term)
			return ports.ToolResult{Err: err}, err
		}
		return ports.ToolResult{Output: map[string]any{"canonical": canon, "matched": true}}, nil

	case ToolListNodes:
		rows, err := r.db.KnowledgeGraphNodesSelectAll()
		if err != nil {
			return ports.ToolResult{Err: err}, err
		}
		ids := make([]string, len(rows))
		for i, row := range rows {
			ids[i] = row.ID
		}
		return ports.ToolResult{Output: map[string]any{"node_ids": ids}}, nil

	case ToolListEdges:
		rows, err := r.db.KnowledgeGraphEdgesSelectAll()
		if err != nil {
			return ports.ToolResult{Err: err}, err
		}
		edges := make([]map[string]any, len(rows))
		for i, e := range rows {
			edges[i] = map[string]any{"from_id": e.FromID, "to_id": e.ToID, "positive": e.Positive}
		}
		return ports.ToolResult{Output: map[string]any{"edges": edges}}, nil

	default:
		err := fmt.Errorf("kg_graph builtin: unknown tool %q", localTool)
		return ports.ToolResult{Err: err}, err
	}
}

func schemaEmpty() map[string]any {
	return map[string]any{
		"type":                 "object",
		"properties":           map[string]any{},
		"additionalProperties": false,
	}
}

func schemaTerm() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"term": map[string]any{
				"type":        "string",
				"description": "Phrase or canonical id to resolve",
			},
		},
		"required":             []any{"term"},
		"additionalProperties": false,
	}
}

func schemaAddNode() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"id": map[string]any{
				"type":        "string",
				"description": "Canonical node id (stored as primary key)",
			},
		},
		"required":             []any{"id"},
		"additionalProperties": false,
	}
}

func schemaAddEdge() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"from_id": map[string]any{
				"type":        "string",
				"description": "Source node id (must exist)",
			},
			"to_id": map[string]any{
				"type":        "string",
				"description": "Target node id (must exist)",
			},
			"positive": map[string]any{
				"type":        "boolean",
				"description": "True for positive correlation, false for negative; omit for true",
			},
		},
		"required":             []any{"from_id", "to_id"},
		"additionalProperties": false,
	}
}

func parseRequiredString(args map[string]any, tool, field string) (string, error) {
	if args == nil {
		return "", fmt.Errorf("kg_graph builtin: %s: arguments required", tool)
	}
	raw, ok := args[field]
	if !ok {
		return "", fmt.Errorf("kg_graph builtin: %s: %s is required", tool, field)
	}
	s, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("kg_graph builtin: %s: %s must be a string", tool, field)
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return "", fmt.Errorf("kg_graph builtin: %s: %s must be non-empty", tool, field)
	}
	return s, nil
}

func parseOptionalPositive(args map[string]any) (bool, error) {
	if args == nil {
		return true, nil
	}
	raw, ok := args["positive"]
	if !ok || raw == nil {
		return true, nil
	}
	switch v := raw.(type) {
	case bool:
		return v, nil
	case float64:
		if v == 0 {
			return false, nil
		}
		if v == 1 {
			return true, nil
		}
		return false, fmt.Errorf("kg_graph builtin: kg_add_edge: positive must be boolean")
	default:
		return false, fmt.Errorf("kg_graph builtin: kg_add_edge: positive must be boolean")
	}
}
