package mcpclient

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/OctoSucker/agent/pkg/ports"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const toolsListCacheTTL = time.Minute

type ToolSpec struct {
	Name        string
	Description string
	InputSchema any
}

type Runner struct {
	mu          sync.RWMutex
	sess        *mcp.ClientSession
	names       map[string]struct{}
	tools       []ToolSpec
	lastRefresh time.Time
}

func Connect(ctx context.Context, endpoint string) (*Runner, error) {
	if endpoint == "" {
		return nil, fmt.Errorf("mcpclient: endpoint is required")
	}
	transport := &mcp.StreamableClientTransport{Endpoint: endpoint}
	r := &Runner{names: make(map[string]struct{})}
	mc := mcp.NewClient(&mcp.Implementation{Name: "octoplus-agent", Version: "0.1"}, nil)
	sess, err := mc.Connect(ctx, transport, nil)
	if err != nil {
		return nil, err
	}
	r.sess = sess
	if err := r.refreshTools(ctx); err != nil {
		_ = sess.Close()
		return nil, err
	}
	return r, nil
}

func (r *Runner) Close() error {
	if r == nil || r.sess == nil {
		return nil
	}
	return r.sess.Close()
}

func (r *Runner) HasTool(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.names[name]
	return ok
}

func (r *Runner) CachedTools() []ToolSpec {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]ToolSpec, len(r.tools))
	copy(out, r.tools)
	return out
}

func (r *Runner) refreshTools(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.names = make(map[string]struct{})
	var all []ToolSpec
	var cursor string
	for {
		res, err := r.sess.ListTools(ctx, &mcp.ListToolsParams{Cursor: cursor})
		if err != nil {
			return fmt.Errorf("mcpclient list_tools: %w", err)
		}
		for _, t := range res.Tools {
			if t == nil || t.Name == "" {
				continue
			}
			r.names[t.Name] = struct{}{}
			all = append(all, ToolSpec{Name: t.Name, Description: t.Description, InputSchema: t.InputSchema})
		}
		if res.NextCursor == "" {
			break
		}
		cursor = res.NextCursor
	}
	r.tools = all
	r.lastRefresh = time.Now()
	return nil
}

func (r *Runner) refreshIfStale(ctx context.Context) error {
	r.mu.RLock()
	stale := time.Since(r.lastRefresh) > toolsListCacheTTL
	r.mu.RUnlock()
	if !stale {
		return nil
	}
	return r.refreshTools(ctx)
}

func (r *Runner) ListCapabilities(ctx context.Context) (map[string]ports.Capability, error) {
	if r == nil || r.sess == nil {
		return nil, fmt.Errorf("mcpclient: runner not connected")
	}
	if err := r.refreshIfStale(ctx); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[string]ports.Capability, len(r.tools))
	for _, t := range r.tools {
		if t.Name == "" {
			continue
		}
		id := t.Name
		out[id] = ports.Capability{ID: id, Tools: []string{id}}
	}
	return out, nil
}

func (r *Runner) Invoke(ctx context.Context, inv ports.CapabilityInvocation) (ports.ToolResult, error) {
	return r.Run(ctx, ports.ToolCall{Name: inv.Tool, Arguments: inv.Arguments})
}

func (r *Runner) Run(ctx context.Context, call ports.ToolCall) (ports.ToolResult, error) {
	if r == nil || r.sess == nil {
		return ports.ToolResult{}, fmt.Errorf("mcpclient: runner not connected")
	}
	if err := r.refreshIfStale(ctx); err != nil {
		log.Printf("mcpclient: list_tools refresh failed before CallTool tool=%q err=%v", call.Name, err)
		return ports.ToolResult{}, err
	}
	if !r.HasTool(call.Name) {
		return ports.ToolResult{}, fmt.Errorf("mcpclient: tool %q not exposed by MCP server (refresh list_tools if hot-plugged)", call.Name)
	}
	args := any(map[string]any{})
	if call.Arguments != nil {
		args = call.Arguments
	}
	res, err := r.sess.CallTool(ctx, &mcp.CallToolParams{Name: call.Name, Arguments: args})
	if err != nil {
		log.Printf("mcpclient: CallTool failed tool=%q arguments=%v err=%v", call.Name, args, err)
		return ports.ToolResult{}, err
	}
	return mcpResultToPorts(res), nil
}
