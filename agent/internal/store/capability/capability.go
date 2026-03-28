package capability

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/OctoSucker/agent/internal/config"
	catalogbuiltin "github.com/OctoSucker/agent/internal/store/capability/builtin/catalog"
	execbuiltin "github.com/OctoSucker/agent/internal/store/capability/builtin/exec"
	skillsbuiltin "github.com/OctoSucker/agent/internal/store/capability/builtin/skills"
	telegrambuiltin "github.com/OctoSucker/agent/internal/store/capability/builtin/telegram"
	mcpstore "github.com/OctoSucker/agent/internal/store/capability/mcp"
	"github.com/OctoSucker/agent/pkg/llmclient"
	"github.com/OctoSucker/agent/pkg/ports"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type CapabilityRegistry struct {
	caps    map[string]ports.Capability
	Runners map[string]CapabilityRunner
}

func NewCapabilityRegistry(
	ctx context.Context,
	mcpEndpoints []string,
	execCfg config.Exec,
	telegramCfg config.Telegram,
	skillsStore *skillsbuiltin.Store,
	plannerLLM *llmclient.OpenAI,
) (*CapabilityRegistry, error) {

	r := &CapabilityRegistry{
		caps:    map[string]ports.Capability{},
		Runners: map[string]CapabilityRunner{},
	}

	// register exec capability
	execRunner, err := execbuiltin.NewRunner(execCfg)
	if err != nil {
		return nil, err
	}
	r.Runners[execbuiltin.CapabilityName] = execRunner

	if strings.TrimSpace(telegramCfg.BotToken) != "" {
		tgRunner, err := telegrambuiltin.NewRunner(telegramCfg)
		if err != nil {
			return nil, fmt.Errorf("capability: telegram builtin: %w", err)
		}
		r.Runners[telegrambuiltin.CapabilityName] = tgRunner
	}

	// register skills capability
	skillsRunner, err := skillsbuiltin.NewRunner(skillsStore, plannerLLM)
	if err != nil {
		return nil, err
	}
	r.Runners[skillsbuiltin.CapabilityName] = skillsRunner

	// register mcp capabilities
	for _, ep := range mcpEndpoints {
		runner, err := mcpstore.NewMcpRunner(ctx, ep)
		if err != nil {
			return nil, fmt.Errorf("capability: connect mcp endpoint %q: %w", ep, err)
		}
		r.Runners[runner.Name()] = runner
	}

	catalogRunner, err := catalogbuiltin.NewRunner(r)
	if err != nil {
		return nil, err
	}
	r.Runners[catalogbuiltin.CapabilityName] = catalogRunner

	for server, runner := range r.Runners {
		tools, err := runner.ToolList(ctx)
		if err != nil {
			return nil, fmt.Errorf("capability: list tool specs for capability %q: %w", server, err)
		}
		toolNames := make([]string, 0, len(tools))
		for _, t := range tools {
			if t.Name == "" {
				continue
			}
			toolNames = append(toolNames, t.Name)
		}
		r.caps[server] = ports.Capability{CapabilityName: server, Tools: toolNames}
	}

	return r, nil
}

// ResyncToolsFromRunners refreshes each capability's tool name list from the current runner ToolList.
// Call after skills reload (or any runner whose ToolList changes) so routing and plan validation match reality.
func (r *CapabilityRegistry) ResyncToolsFromRunners(ctx context.Context) error {
	for server, runner := range r.Runners {
		tools, err := runner.ToolList(ctx)
		if err != nil {
			return fmt.Errorf("capability: resync tool list for %q: %w", server, err)
		}
		toolNames := make([]string, 0, len(tools))
		for _, t := range tools {
			if t == nil || t.Name == "" {
				continue
			}
			toolNames = append(toolNames, t.Name)
		}
		r.caps[server] = ports.Capability{CapabilityName: server, Tools: toolNames}
	}
	return nil
}

func (r *CapabilityRegistry) Invoke(ctx context.Context, inv ports.CapabilityInvocation) (ports.ToolResult, error) {
	runner, ok := r.Runners[inv.CapabilityName]
	if !ok {
		return ports.ToolResult{}, fmt.Errorf("capability: no runner for capability %q", inv.CapabilityName)
	}
	return runner.Invoke(ctx, inv)
}

func (r *CapabilityRegistry) AllCapabilities() map[string]ports.Capability {
	out := make(map[string]ports.Capability, len(r.caps))
	for k, v := range r.caps {
		out[k] = ports.Capability{CapabilityName: v.CapabilityName, Tools: append([]string(nil), v.Tools...)}
	}
	return out
}

func (r *CapabilityRegistry) Tools(capID string) ([]string, error) {
	capabilityDef, ok := r.caps[capID]
	if !ok {
		return nil, fmt.Errorf("capability: capability %q not found", capID)
	}
	return append([]string(nil), capabilityDef.Tools...), nil
}

func (r *CapabilityRegistry) Tool(ctx context.Context, capID, tool string) (*mcp.Tool, error) {
	runner, ok := r.Runners[capID]
	if !ok {
		return nil, fmt.Errorf("capability: no runner for capability %q", capID)
	}
	t, err := runner.Tool(tool)
	if err != nil {
		return nil, fmt.Errorf("capability: get tool for tool %q: %w", tool, err)
	}
	return t, nil
}

func (r *CapabilityRegistry) PlannerToolAppendix(ctx context.Context) string {
	var b strings.Builder
	b.WriteString("Tools (names must match tool calls):\n")
	for capName, capDef := range r.caps {
		for _, toolName := range capDef.Tools {
			b.WriteString("- ")
			b.WriteString(toolName)
			b.WriteString(" [capability=")
			b.WriteString(capName)
			b.WriteString("]")
			if t, err := r.Tool(ctx, capName, toolName); err == nil {
				raw, err := json.Marshal(t.InputSchema)
				if err != nil {
					return ""
				}
				b.WriteString(" params JSON Schema: ")
				b.Write(raw)
			}
			b.WriteByte('\n')
		}
	}
	if _, ok := r.Runners[execbuiltin.CapabilityName]; ok {
		raw, err := json.Marshal(execbuiltin.ToolInputSchema())
		if err != nil {
			return ""
		}
		b.WriteString("- <argv0: any program name available in the sandbox image> [capability=")
		b.WriteString(execbuiltin.CapabilityName)
		b.WriteString("] params JSON Schema: ")
		b.Write(raw)
		b.WriteByte('\n')
	}
	return b.String()
}
