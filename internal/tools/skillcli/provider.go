package skillcli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/OctoSucker/octosucker/config"
	types "github.com/OctoSucker/octosucker/internal/runtime/model"
	skillsbuiltin "github.com/OctoSucker/octosucker/internal/tools/builtin/skills"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Provider struct {
	workspaceRoot string
	openAI        config.OpenAI
	skill         skillsbuiltin.SkillMeta
	spec          *skillsbuiltin.CLIPluginSpec
	toolsByName   map[string]skillsbuiltin.CLIPluginTool
}

func NewProvider(workspaceRoot string, openAI config.OpenAI, skill skillsbuiltin.SkillMeta) (*Provider, error) {
	if skill.CLIPlugin == nil {
		return nil, fmt.Errorf("skill cli provider: skill %q has no cli plugin", skill.Name)
	}
	spec := skill.CLIPlugin
	if strings.TrimSpace(workspaceRoot) == "" {
		return nil, fmt.Errorf("skill cli provider: workspace root is required")
	}
	p := &Provider{
		workspaceRoot: workspaceRoot,
		openAI:        openAI,
		skill:         skill,
		spec:          spec,
		toolsByName:   make(map[string]skillsbuiltin.CLIPluginTool, len(spec.Tools)),
	}
	for _, tool := range spec.Tools {
		if _, exists := p.toolsByName[tool.Name]; exists {
			return nil, fmt.Errorf("skill cli provider: duplicate tool %q in skill %q", tool.Name, skill.Name)
		}
		p.toolsByName[tool.Name] = tool
	}
	return p, nil
}

func (p *Provider) Name() (string, string) {
	desc := strings.TrimSpace(p.skill.Description)
	if desc == "" {
		desc = fmt.Sprintf("CLI plugin from skill %q", p.skill.Name)
	}
	return p.spec.Provider, desc
}

func (p *Provider) HasTool(name string) bool {
	_, ok := p.toolsByName[strings.TrimSpace(name)]
	return ok
}

func (p *Provider) Tool(tool string) (*mcp.Tool, error) {
	def, ok := p.toolsByName[strings.TrimSpace(tool)]
	if !ok {
		return nil, fmt.Errorf("skill cli provider: unknown tool %q", tool)
	}
	schema := def.InputSchema
	if schema == nil {
		schema = map[string]any{
			"type":                 "object",
			"properties":           map[string]any{},
			"additionalProperties": false,
		}
	}
	return &mcp.Tool{
		Name:        def.Name,
		Description: def.Description,
		InputSchema: schema,
	}, nil
}

func (p *Provider) ToolList(ctx context.Context) ([]*mcp.Tool, error) {
	names := make([]string, 0, len(p.toolsByName))
	for name := range p.toolsByName {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]*mcp.Tool, 0, len(names))
	for _, name := range names {
		t, err := p.Tool(name)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, nil
}

func (p *Provider) Invoke(ctx context.Context, localTool string, arguments map[string]any) (types.ToolResult, error) {
	def, ok := p.toolsByName[strings.TrimSpace(localTool)]
	if !ok {
		err := fmt.Errorf("skill cli provider: unknown tool %q", localTool)
		return types.ToolResult{Err: err}, err
	}
	argsJSON, err := json.Marshal(arguments)
	if err != nil {
		err = fmt.Errorf("skill cli provider: marshal args: %w", err)
		return types.ToolResult{Err: err}, err
	}
	cmd := exec.CommandContext(ctx, p.spec.Command, p.renderArgs(def.Args, def.Name, string(argsJSON))...)
	if wd := p.renderString(p.spec.WorkDir, def.Name, string(argsJSON)); strings.TrimSpace(wd) != "" {
		if filepath.IsAbs(wd) {
			cmd.Dir = filepath.Clean(wd)
		} else {
			cmd.Dir = filepath.Clean(filepath.Join(p.workspaceRoot, wd))
		}
	}
	cmd.Env = p.renderEnv(def.Name, string(argsJSON))

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		callErr := extractCLIError(stdout.Bytes(), stderr.String(), err)
		return types.ToolResult{Err: callErr}, callErr
	}

	var output map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
		parseErr := fmt.Errorf("skill cli provider: parse cli output: %w", err)
		return types.ToolResult{Err: parseErr}, parseErr
	}
	if msg, ok := output["error"].(string); ok && strings.TrimSpace(msg) != "" {
		callErr := fmt.Errorf("skill cli provider: %s", msg)
		return types.ToolResult{Err: callErr}, callErr
	}
	return types.ToolResult{Output: output}, nil
}

func (p *Provider) renderArgs(args []string, toolName, argsJSON string) []string {
	out := make([]string, 0, len(args))
	for _, arg := range args {
		out = append(out, p.renderString(arg, toolName, argsJSON))
	}
	return out
}

func (p *Provider) renderEnv(toolName, argsJSON string) []string {
	out := append([]string(nil), os.Environ()...)
	for key, value := range p.spec.Env {
		rendered := p.renderString(value, toolName, argsJSON)
		if strings.TrimSpace(rendered) == "" {
			continue
		}
		out = append(out, key+"="+rendered)
	}
	return out
}

func (p *Provider) renderString(in, toolName, argsJSON string) string {
	replacer := strings.NewReplacer(
		"{{workspace_root}}", p.workspaceRoot,
		"{{tool_name}}", toolName,
		"{{args_json}}", argsJSON,
		"{{openai_api_key}}", p.openAI.APIKey,
		"{{openai_base_url}}", p.openAI.BaseURL,
		"{{openai_model}}", p.openAI.Model,
		"{{openai_embedding_model}}", p.openAI.EmbeddingModel,
	)
	return replacer.Replace(in)
}

func extractCLIError(stdout []byte, stderr string, runErr error) error {
	var payload map[string]any
	if len(stdout) > 0 && json.Unmarshal(stdout, &payload) == nil {
		if msg, ok := payload["error"].(string); ok && strings.TrimSpace(msg) != "" {
			return fmt.Errorf("skill cli provider: %s", msg)
		}
	}
	if msg := strings.TrimSpace(stderr); msg != "" {
		return fmt.Errorf("skill cli provider: %s", msg)
	}
	return fmt.Errorf("skill cli provider: %w", runErr)
}
