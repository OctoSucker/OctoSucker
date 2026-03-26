package mcpclient

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"

	"github.com/OctoSucker/agent/pkg/ports"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Runner struct {
	name  string
	mu    sync.RWMutex
	sess  *mcp.ClientSession
	names map[string]struct{}
	tools []mcp.Tool
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
	init := sess.InitializeResult()
	if init == nil || init.ServerInfo == nil || init.ServerInfo.Name == "" {
		if cerr := sess.Close(); cerr != nil {
			err = errors.Join(err, fmt.Errorf("mcpclient: close session after missing server info: %w", cerr))
		}
		return nil, fmt.Errorf("mcpclient: initialize result missing server name")
	}
	r.name = init.ServerInfo.Name
	r.mu.Lock()
	r.names = make(map[string]struct{})
	var all []mcp.Tool
	var cursor string
	for {
		res, err := r.sess.ListTools(ctx, &mcp.ListToolsParams{Cursor: cursor})
		if err != nil {
			r.mu.Unlock()
			if cerr := sess.Close(); cerr != nil {
				err = errors.Join(err, fmt.Errorf("mcpclient: close session after refresh failure: %w", cerr))
			}
			return nil, fmt.Errorf("mcpclient list_tools: %w", err)
		}
		for _, t := range res.Tools {
			if t == nil || t.Name == "" {
				continue
			}
			r.names[t.Name] = struct{}{}
			all = append(all, *t)
		}
		if res.NextCursor == "" {
			break
		}
		cursor = res.NextCursor
	}
	r.tools = all
	r.mu.Unlock()
	return r, nil
}

func (r *Runner) Close() error {
	return r.sess.Close()
}

func (r *Runner) Name() string {

	return r.name
}

func (r *Runner) HasTool(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.names[name]
	return ok
}

func (r *Runner) ListToolSpecs(ctx context.Context) []mcp.Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]mcp.Tool, len(r.tools))
	copy(out, r.tools)
	return out
}

func (r *Runner) Invoke(ctx context.Context, inv ports.CapabilityInvocation) (ports.ToolResult, error) {
	r.mu.Lock()
	r.names = make(map[string]struct{})
	var all []mcp.Tool
	var cursor string
	for {
		res, err := r.sess.ListTools(ctx, &mcp.ListToolsParams{Cursor: cursor})
		if err != nil {
			r.mu.Unlock()
			log.Printf("mcpclient: list_tools refresh failed before CallTool tool=%q err=%v", inv.Tool, err)
			return ports.ToolResult{}, fmt.Errorf("mcpclient list_tools: %w", err)
		}
		for _, t := range res.Tools {
			if t == nil || t.Name == "" {
				continue
			}
			r.names[t.Name] = struct{}{}
			all = append(all, *t)
		}
		if res.NextCursor == "" {
			break
		}
		cursor = res.NextCursor
	}
	r.tools = all
	_, ok := r.names[inv.Tool]
	r.mu.Unlock()
	if !ok {
		return ports.ToolResult{}, fmt.Errorf("mcpclient: tool %q not exposed by MCP server (refresh list_tools if hot-plugged)", inv.Tool)
	}
	args := any(map[string]any{})
	if inv.Arguments != nil {
		args = inv.Arguments
	}
	res, err := r.sess.CallTool(ctx, &mcp.CallToolParams{Name: inv.Tool, Arguments: args})
	if err != nil {
		log.Printf("mcpclient: CallTool failed tool=%q arguments=%v err=%v", inv.Tool, args, err)
		return ports.ToolResult{}, err
	}
	return mcpResultToPorts(res), nil
}
