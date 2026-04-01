package mcp

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/OctoSucker/agent/pkg/ports"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Runner struct {
	name  string
	mu    sync.RWMutex
	sess  *mcp.ClientSession
	Tools map[string]*mcp.Tool
}

func NewMcpRunner(ctx context.Context, endpoint string) (*Runner, error) {
	if endpoint == "" {
		return nil, fmt.Errorf("mcp: endpoint is required")
	}
	r := &Runner{
		Tools: make(map[string]*mcp.Tool),
	}
	mc := mcp.NewClient(&mcp.Implementation{Name: "octoplus-agent", Version: "0.1"}, nil)
	sess, err := mc.Connect(
		ctx,
		&mcp.StreamableClientTransport{Endpoint: endpoint},
		nil,
	)
	if err != nil {
		return nil, err
	}
	r.sess = sess
	r.name = sess.InitializeResult().ServerInfo.Name
	return r, nil
}

func (r *Runner) Name() string { return r.name }

func (r *Runner) ToolList(ctx context.Context) ([]*mcp.Tool, error) {
	r.Tools = make(map[string]*mcp.Tool)
	var cursor string
	for {
		res, err := r.sess.ListTools(ctx, &mcp.ListToolsParams{Cursor: cursor})
		if err != nil {
			return nil, err
		}
		for _, t := range res.Tools {
			if t == nil || t.Name == "" {
				continue
			}
			r.Tools[t.Name] = t
		}
		if res.NextCursor == "" {
			break
		}
		cursor = res.NextCursor
	}
	var tools []*mcp.Tool
	for tool := range r.Tools {
		tools = append(tools, r.Tools[tool])
	}
	return tools, nil
}

func (r *Runner) HasTool(tool string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if len(r.Tools) == 0 {
		r.ToolList(context.Background())
	}
	_, ok := r.Tools[tool]
	return ok
}

func (r *Runner) Tool(tool string) (*mcp.Tool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if len(r.Tools) == 0 {
		r.ToolList(context.Background())
	}
	t, ok := r.Tools[tool]
	if !ok {
		return nil, fmt.Errorf("mcp: tool %q not exposed by MCP server", tool)
	}
	return t, nil
}

func (r *Runner) Invoke(ctx context.Context, inv ports.CapabilityInvocation) (ports.ToolResult, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ok := r.HasTool(inv.Tool)
	if !ok {
		return ports.ToolResult{Err: fmt.Errorf("mcp: tool %q not exposed by MCP server", inv.Tool)}, fmt.Errorf("mcp: tool %q not exposed by MCP server", inv.Tool)
	}

	res, err := r.sess.CallTool(ctx, &mcp.CallToolParams{Name: inv.Tool, Arguments: inv.Arguments})
	if err != nil {
		log.Printf("mcp: CallTool failed tool=%q arguments=%v err=%v", inv.Tool, inv.Arguments, err)
		return ports.ToolResult{Err: err}, err
	}
	if res.IsError {
		msg := joinTextContent(res.Content)
		if msg == "" {
			msg = "tool error"
		}
		return ports.ToolResult{Output: msg, Err: errors.New(msg)}, nil
	}
	if res.StructuredContent != nil {
		return ports.ToolResult{Output: fmt.Sprintf("%v", res.StructuredContent)}, nil
	}
	return ports.ToolResult{Output: joinTextContent(res.Content)}, nil
}

func joinTextContent(content []mcp.Content) string {
	var parts []string
	for _, c := range content {
		if tc, ok := c.(*mcp.TextContent); ok {
			parts = append(parts, tc.Text)
		} else {
			parts = append(parts, fmt.Sprint(c))
		}
	}
	return strings.Join(parts, "\n")
}
