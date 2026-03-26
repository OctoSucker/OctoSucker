package mcpclient

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/OctoSucker/agent/pkg/ports"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type MCPRouter struct {
	runners map[string]*Runner
}

func NewMCPRouter(ctx context.Context, endpoints []string) (*MCPRouter, func(), error) {
	if len(endpoints) == 0 {
		return nil, nil, fmt.Errorf("mcpclient: endpoints are empty")
	}
	runners := make(map[string]*Runner)
	for _, ep := range endpoints {
		r, err := Connect(ctx, ep)
		if err != nil {
			var closeErr error
			for _, x := range runners {
				if cerr := x.Close(); cerr != nil {
					closeErr = errors.Join(closeErr, cerr)
				}
			}
			if closeErr != nil {
				err = errors.Join(err, fmt.Errorf("mcpclient.ConnectMCPRouter: cleanup: %w", closeErr))
			}
			return nil, nil, fmt.Errorf("mcpclient.ConnectMCPRouter %q: %w", ep, err)
		}
		runners[r.Name()] = r
	}
	router := &MCPRouter{runners: runners}

	return router, func() {
		if err := router.Close(); err != nil {
			log.Printf("mcpclient: router close: %v", err)
		}
	}, nil
}

func (m *MCPRouter) ListToolSpecs() []mcp.Tool {
	if m == nil {
		return nil
	}
	var out []mcp.Tool
	for _, r := range m.runners {
		out = append(out, r.ListToolSpecs(context.Background())...)
	}
	return out
}

func (m *MCPRouter) HasTool(name string) bool {
	if m == nil {
		return false
	}
	for _, r := range m.runners {
		if r.HasTool(name) {
			return true
		}
	}
	return false
}

func (m *MCPRouter) Close() error {
	if m == nil {
		return nil
	}
	var first error
	for _, r := range m.runners {
		if err := r.Close(); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// key: server name, value: tools
func (m *MCPRouter) ListCapabilities(ctx context.Context) (map[string][]mcp.Tool, error) {
	if m == nil || len(m.runners) == 0 {
		return nil, fmt.Errorf("mcpclient.MCPRouter: empty")
	}
	out := make(map[string][]mcp.Tool)
	for _, r := range m.runners {
		tools := r.ListToolSpecs(ctx)
		out[r.Name()] = tools
	}
	return out, nil
}

func (m *MCPRouter) Invoke(ctx context.Context, inv ports.CapabilityInvocation) (ports.ToolResult, error) {
	if m == nil {
		return ports.ToolResult{}, fmt.Errorf("mcpclient.MCPRouter: nil")
	}
	runner, ok := m.runners[inv.CapabilityName]
	if !ok {
		return ports.ToolResult{}, fmt.Errorf("mcpclient.MCPRouter: server %q not found", inv.CapabilityName)
	}
	if !runner.HasTool(inv.Tool) {
		return ports.ToolResult{}, fmt.Errorf("mcpclient.MCPRouter: tool %q not found on server %q", inv.Tool, inv.CapabilityName)
	}
	return runner.Invoke(ctx, inv)
}
