package mcpclient

import (
	"context"
	"fmt"
	"strings"

	"github.com/OctoSucker/agent/pkg/ports"
)

type MCPRouter struct {
	runners []*Runner
}

func SplitEndpoints(raw string) []string {
	if raw == "" {
		return nil
	}
	var out []string
	for _, part := range strings.Split(raw, ",") {
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func ConnectMCPRouter(ctx context.Context, endpoints []string) (*MCPRouter, error) {
	if len(endpoints) == 0 {
		return nil, fmt.Errorf("mcpclient.ConnectMCPRouter: no endpoints")
	}
	var runners []*Runner
	for _, ep := range endpoints {
		r, err := Connect(ctx, ep)
		if err != nil {
			for _, x := range runners {
				_ = x.Close()
			}
			return nil, fmt.Errorf("mcpclient.ConnectMCPRouter %q: %w", ep, err)
		}
		runners = append(runners, r)
	}
	return &MCPRouter{runners: runners}, nil
}

func (m *MCPRouter) CachedToolSpecs() []ToolSpec {
	if m == nil {
		return nil
	}
	var out []ToolSpec
	for _, r := range m.runners {
		out = append(out, r.CachedTools()...)
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

func (m *MCPRouter) ListCapabilities(ctx context.Context) (map[string]ports.Capability, error) {
	if m == nil || len(m.runners) == 0 {
		return nil, fmt.Errorf("mcpclient.MCPRouter: empty")
	}
	out := make(map[string]ports.Capability)
	for _, r := range m.runners {
		caps, err := r.ListCapabilities(ctx)
		if err != nil {
			return nil, err
		}
		for id, cap := range caps {
			if _, dup := out[id]; dup {
				return nil, fmt.Errorf("mcpclient.MCPRouter: duplicate tool %q (use one MCP per tool name)", id)
			}
			out[id] = cap
		}
	}
	return out, nil
}

func (m *MCPRouter) Invoke(ctx context.Context, inv ports.CapabilityInvocation) (ports.ToolResult, error) {
	if m == nil {
		return ports.ToolResult{}, fmt.Errorf("mcpclient.MCPRouter: nil")
	}
	for _, r := range m.runners {
		if err := r.refreshIfStale(ctx); err != nil {
			return ports.ToolResult{}, err
		}
		if r.HasTool(inv.Tool) {
			return r.Invoke(ctx, inv)
		}
	}
	return ports.ToolResult{}, fmt.Errorf("mcpclient.MCPRouter: tool %q not on any endpoint", inv.Tool)
}

func ConnectForApp(ctx context.Context, endpoint string) (*MCPRouter, func(), error) {
	eps := SplitEndpoints(endpoint)
	if len(eps) == 0 {
		return nil, nil, fmt.Errorf("mcpclient: endpoint is empty")
	}
	router, err := ConnectMCPRouter(ctx, eps)
	if err != nil {
		return nil, nil, err
	}
	return router, func() { _ = router.Close() }, nil
}
