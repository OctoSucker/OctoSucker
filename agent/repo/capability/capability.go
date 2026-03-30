package capability

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/OctoSucker/agent/internal/config"
	"github.com/OctoSucker/agent/pkg/ports"
	catalogbuiltin "github.com/OctoSucker/agent/repo/capability/builtin/catalog"
	execbuiltin "github.com/OctoSucker/agent/repo/capability/builtin/exec"
	skillsbuiltin "github.com/OctoSucker/agent/repo/capability/builtin/skills"
	telegrambuiltin "github.com/OctoSucker/agent/repo/capability/builtin/telegram"
	mcpstore "github.com/OctoSucker/agent/repo/capability/mcp"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type CapabilityRegistry struct {
	caps         map[string]ports.Capability
	Runners      map[string]CapabilityRunner
	skillsRunner *skillsbuiltin.Runner
}

func NewCapabilityRegistry(
	ctx context.Context,
	mcpEndpoints []string,
	execCfg config.Exec,
	telegramCfg config.Telegram,
	skillsRunner *skillsbuiltin.Runner,
	catalogRunner *catalogbuiltin.Runner,
) (*CapabilityRegistry, error) {

	r := &CapabilityRegistry{
		caps:         map[string]ports.Capability{},
		Runners:      map[string]CapabilityRunner{},
		skillsRunner: skillsRunner,
	}

	// register exec capability
	execRunner, err := execbuiltin.NewRunner(execCfg)
	if err != nil {
		return nil, err
	}
	r.Runners[execRunner.Name()] = execRunner

	if strings.TrimSpace(telegramCfg.BotToken) != "" {
		tgRunner, err := telegrambuiltin.NewRunner(telegramCfg)
		if err != nil {
			return nil, fmt.Errorf("capability: telegram builtin: %w", err)
		}
		r.Runners[telegrambuiltin.CapabilityName] = tgRunner
	}

	if skillsRunner == nil {
		return nil, fmt.Errorf("capability: skills runner is required")
	}
	r.Runners[skillsbuiltin.CapabilityName] = skillsRunner

	if catalogRunner == nil {
		return nil, fmt.Errorf("capability: catalog runner is required")
	}
	r.Runners[catalogbuiltin.CapabilityName] = catalogRunner

	// register mcp capabilities
	for _, ep := range mcpEndpoints {
		runner, err := mcpstore.NewMcpRunner(ctx, ep)
		if err != nil {
			return nil, fmt.Errorf("capability: connect mcp endpoint %q: %w", ep, err)
		}
		r.Runners[runner.Name()] = runner
	}

	r.Runners[RegistryCapabilityName] = newRegistryBuiltinRunner(r)

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

func (r *CapabilityRegistry) PlannerSkills() skillsbuiltin.PromptBundle {
	if r == nil || r.skillsRunner == nil {
		return skillsbuiltin.PromptBundle{}
	}
	return r.skillsRunner.PlannerBundle()
}

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

func (r *CapabilityRegistry) Tool(capID, tool string) (*mcp.Tool, error) {
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

func (r *CapabilityRegistry) PlannerToolAppendix() string {
	var b strings.Builder
	b.WriteString("Tools (names must match tool calls):\n")
	for capName, capDef := range r.caps {
		for _, toolName := range capDef.Tools {
			b.WriteString("- ")
			b.WriteString(toolName)
			b.WriteString(" [capability=")
			b.WriteString(capName)
			b.WriteString("]")
			if t, err := r.Tool(capName, toolName); err == nil {
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
	if r.skillsRunner != nil {
		bound, err := r.skillsRunner.BoundMcpTools()
		if err != nil {
			return ""
		}
		for _, t := range bound {
			if t == nil || t.Name == "" {
				continue
			}
			b.WriteString("- ")
			b.WriteString(t.Name)
			b.WriteString(" [capability=")
			b.WriteString(skillsbuiltin.CapabilityName)
			b.WriteString("]")
			raw, err := json.Marshal(t.InputSchema)
			if err != nil {
				return ""
			}
			b.WriteString(" params JSON Schema: ")
			b.Write(raw)
			b.WriteByte('\n')
		}
	}
	return b.String()
}
