package kggraph

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/OctoSucker/octosucker/engine/types"
	"github.com/OctoSucker/octosucker/pkg/llmclient"
	"github.com/OctoSucker/octosucker/repo/knowledge"
	"github.com/OctoSucker/octosucker/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	ToolAddEdge            = "kg_add_edge"
	ToolAddEdgesBatch      = "kg_add_edges_batch"
	ToolLookupNodeExact    = "kg_lookup_node_exact"
	ToolLookupNodeSemantic = "kg_lookup_node_semantic"
	ToolListNodes          = "kg_list_nodes"
	ToolListEdges          = "kg_list_edges"
)

// Runner runs knowledge-graph tools against workspace SQLite opened from workspaceRoot (implements tools.Provider).
type Runner struct {
	db *store.DB
	g  *knowledge.Graph
}

// NewRunner opens <workspaceRoot>/data/octosucker.sqlite and builds the graph with llm for embeddings.
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
	db, err := store.Open(abs)
	if err != nil {
		return nil, fmt.Errorf("kg_graph builtin: open agent db: %w", err)
	}
	g, err := knowledge.New(db, llm)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("kg_graph builtin: open graph: %w", err)
	}
	return &Runner{db: db, g: g}, nil
}

// Name is the stable tool-provider name (Registry.providersByName key); not an MCP tool name.
func (r *Runner) Name() (string, string) {
	return "kg_graph", "Workspace knowledge graph in SQLite: nodes, edges, semantic lookup."
}

func (r *Runner) HasTool(name string) bool {
	switch strings.TrimSpace(name) {
	case ToolAddEdge, ToolAddEdgesBatch, ToolLookupNodeExact, ToolLookupNodeSemantic, ToolListNodes, ToolListEdges:
		return true
	default:
		return false
	}
}

func (r *Runner) Tool(tool string) (*mcp.Tool, error) {
	switch strings.TrimSpace(tool) {
	case ToolAddEdge:
		return &mcp.Tool{
			Name:        ToolAddEdge,
			Description: "Add a directed influence edge from_id → to_id. Creates both endpoint nodes (with embeddings) if they do not exist. Use positive=false for negative correlation.",
			InputSchema: schemaAddEdge(),
		}, nil
	case ToolAddEdgesBatch:
		return &mcp.Tool{
			Name:        ToolAddEdgesBatch,
			Description: "Add multiple directed influence edges. Each edge creates endpoint nodes (with embeddings) if missing.",
			InputSchema: schemaAddEdgesBatch(),
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
	names := []string{ToolAddEdge, ToolAddEdgesBatch, ToolLookupNodeExact, ToolLookupNodeSemantic, ToolListNodes, ToolListEdges}
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

func (r *Runner) Invoke(ctx context.Context, localTool string, arguments map[string]any) (types.ToolResult, error) {
	if r == nil || r.db == nil || r.g == nil {
		return types.ToolResult{Err: fmt.Errorf("kg_graph builtin: not initialized")}, fmt.Errorf("kg_graph builtin: not initialized")
	}
	switch localTool {
	case ToolAddEdge:
		fromID, err := parseRequiredString(arguments, ToolAddEdge, "from_id")
		if err != nil {
			return types.ToolResult{Err: err}, err
		}
		toID, err := parseRequiredString(arguments, ToolAddEdge, "to_id")
		if err != nil {
			return types.ToolResult{Err: err}, err
		}
		positive, err := parseOptionalPositive(arguments)
		if err != nil {
			return types.ToolResult{Err: err}, err
		}
		if err := r.g.EnsureNode(ctx, fromID); err != nil {
			return types.ToolResult{Err: err}, err
		}
		if err := r.g.EnsureNode(ctx, toID); err != nil {
			return types.ToolResult{Err: err}, err
		}
		if err := r.g.AddEdge(fromID, toID, positive); err != nil {
			return types.ToolResult{Err: err}, err
		}
		return types.ToolResult{Output: map[string]any{"from_id": fromID, "to_id": toID, "positive": positive}}, nil

	case ToolAddEdgesBatch:
		edges, err := parseBatchEdges(arguments)
		if err != nil {
			return types.ToolResult{Err: err}, err
		}
		added := make([]map[string]any, 0, len(edges))
		for i, e := range edges {
			if err := r.g.EnsureNode(ctx, e.FromID); err != nil {
				wrapped := fmt.Errorf("kg_graph builtin: %s: edge[%d]: ensure from_id %q: %w", ToolAddEdgesBatch, i, e.FromID, err)
				return types.ToolResult{Err: wrapped}, wrapped
			}
			if err := r.g.EnsureNode(ctx, e.ToID); err != nil {
				wrapped := fmt.Errorf("kg_graph builtin: %s: edge[%d]: ensure to_id %q: %w", ToolAddEdgesBatch, i, e.ToID, err)
				return types.ToolResult{Err: wrapped}, wrapped
			}
			if err := r.g.AddEdge(e.FromID, e.ToID, e.Positive); err != nil {
				wrapped := fmt.Errorf("kg_graph builtin: %s: edge[%d]: add edge %q -> %q: %w", ToolAddEdgesBatch, i, e.FromID, e.ToID, err)
				return types.ToolResult{Err: wrapped}, wrapped
			}
			added = append(added, map[string]any{
				"from_id":  e.FromID,
				"to_id":    e.ToID,
				"positive": e.Positive,
			})
		}
		return types.ToolResult{Output: map[string]any{
			"added_count": len(added),
			"edges":       added,
		}}, nil

	case ToolLookupNodeExact:
		term, err := parseRequiredString(arguments, ToolLookupNodeExact, "term")
		if err != nil {
			return types.ToolResult{Err: err}, err
		}
		canon, ok, err := r.g.CanonicalFor(term)
		if err != nil {
			return types.ToolResult{Err: err}, err
		}
		if !ok {
			err := fmt.Errorf("kg_graph builtin: %s: no exact match for %q", ToolLookupNodeExact, term)
			return types.ToolResult{Err: err}, err
		}
		return types.ToolResult{Output: map[string]any{"canonical": canon, "matched": true}}, nil

	case ToolLookupNodeSemantic:
		term, err := parseRequiredString(arguments, ToolLookupNodeSemantic, "term")
		if err != nil {
			return types.ToolResult{Err: err}, err
		}
		canon, ok, err := r.g.CanonicalForContext(ctx, term)
		if err != nil {
			return types.ToolResult{Err: err}, err
		}
		if !ok {
			err := fmt.Errorf("kg_graph builtin: %s: no match for %q", ToolLookupNodeSemantic, term)
			return types.ToolResult{Err: err}, err
		}
		return types.ToolResult{Output: map[string]any{"canonical": canon, "matched": true}}, nil

	case ToolListNodes:
		rows, err := r.db.KnowledgeGraphNodesSelectAll()
		if err != nil {
			return types.ToolResult{Err: err}, err
		}
		ids := make([]string, len(rows))
		for i, row := range rows {
			ids[i] = row.ID
		}
		return types.ToolResult{Output: map[string]any{"node_ids": ids}}, nil

	case ToolListEdges:
		rows, err := r.db.KnowledgeGraphEdgesSelectAll()
		if err != nil {
			return types.ToolResult{Err: err}, err
		}
		edges := make([]map[string]any, len(rows))
		for i, e := range rows {
			edges[i] = map[string]any{"from_id": e.FromID, "to_id": e.ToID, "positive": e.Positive}
		}
		return types.ToolResult{Output: map[string]any{"edges": edges}}, nil

	default:
		err := fmt.Errorf("kg_graph builtin: unknown tool %q", localTool)
		return types.ToolResult{Err: err}, err
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
		"additionalProperties": false,
	}
}

func schemaAddEdge() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"from_id": map[string]any{
				"type":        "string",
				"description": "Source node id; created with embedding if missing",
			},
			"to_id": map[string]any{
				"type":        "string",
				"description": "Target node id; created with embedding if missing",
			},
			"positive": map[string]any{
				"type":        "boolean",
				"description": "True for positive correlation, false for negative; omit for true",
			},
		},
		"required":             []string{"from_id", "to_id"},
		"additionalProperties": false,
	}
}

func schemaAddEdgesBatch() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"edges": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"from_id": map[string]any{
							"type":        "string",
							"description": "Source node id; created with embedding if missing",
						},
						"to_id": map[string]any{
							"type":        "string",
							"description": "Target node id; created with embedding if missing",
						},
						"positive": map[string]any{
							"type":        "boolean",
							"description": "True for positive correlation, false for negative; omit for true",
						},
					},
					"required":             []string{"from_id", "to_id"},
					"additionalProperties": false,
				},
				"minItems": 1,
			},
		},
		"required":             []string{"edges"},
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

type batchEdge struct {
	FromID   string
	ToID     string
	Positive bool
}

func parseBatchEdges(args map[string]any) ([]batchEdge, error) {
	if args == nil {
		return nil, fmt.Errorf("kg_graph builtin: %s: arguments required", ToolAddEdgesBatch)
	}
	rawEdges, ok := args["edges"]
	if !ok {
		return nil, fmt.Errorf("kg_graph builtin: %s: edges is required", ToolAddEdgesBatch)
	}
	edgeItems, ok := rawEdges.([]any)
	if !ok {
		return nil, fmt.Errorf("kg_graph builtin: %s: edges must be an array", ToolAddEdgesBatch)
	}
	if len(edgeItems) == 0 {
		return nil, fmt.Errorf("kg_graph builtin: %s: edges must be non-empty", ToolAddEdgesBatch)
	}
	edges := make([]batchEdge, 0, len(edgeItems))
	for i, raw := range edgeItems {
		obj, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("kg_graph builtin: %s: edge[%d] must be an object", ToolAddEdgesBatch, i)
		}
		fromID, err := parseRequiredString(obj, ToolAddEdgesBatch, "from_id")
		if err != nil {
			return nil, fmt.Errorf("kg_graph builtin: %s: edge[%d]: %w", ToolAddEdgesBatch, i, err)
		}
		toID, err := parseRequiredString(obj, ToolAddEdgesBatch, "to_id")
		if err != nil {
			return nil, fmt.Errorf("kg_graph builtin: %s: edge[%d]: %w", ToolAddEdgesBatch, i, err)
		}
		positive, err := parseOptionalPositive(obj)
		if err != nil {
			return nil, fmt.Errorf("kg_graph builtin: %s: edge[%d]: %w", ToolAddEdgesBatch, i, err)
		}
		edges = append(edges, batchEdge{FromID: fromID, ToID: toID, Positive: positive})
	}
	return edges, nil
}
