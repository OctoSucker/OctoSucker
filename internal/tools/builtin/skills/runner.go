package skillsbuiltin

import (
	"context"
	"fmt"
	"strings"

	"github.com/OctoSucker/octosucker/internal/skills"
	types "github.com/OctoSucker/octosucker/internal/toolcontract"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	ProviderName      = "skills"
	ToolGetRootDir    = "get_skills_root_dir"
	ToolListSkills    = "list_skills"
	ToolActivateSkill = "activate_skill"
	ToolReadResource  = "read_skill_resource"
)

// Runner exposes a validated Agent Skills catalog through MCP-shaped tools.
type Runner struct {
	catalog *skills.Catalog
}

func NewRunner(dir string) (*Runner, error) {
	catalog, err := skills.NewCatalog(dir)
	if err != nil {
		return nil, err
	}
	return &Runner{catalog: catalog}, nil
}

func (r *Runner) Name() (string, string) {
	return ProviderName, "Directory-based Agent Skills with explicit activation and on-demand resources."
}

func (r *Runner) RootDir() string {
	if r == nil || r.catalog == nil {
		return ""
	}
	return r.catalog.Root()
}

func (r *Runner) AllSkills() []skills.Skill {
	if r == nil || r.catalog == nil {
		return nil
	}
	return r.catalog.All()
}

func (r *Runner) HasTool(name string) bool {
	switch strings.TrimSpace(name) {
	case ToolGetRootDir, ToolListSkills, ToolActivateSkill, ToolReadResource:
		return true
	default:
		return false
	}
}

func (r *Runner) Tool(name string) (*mcp.Tool, error) {
	for _, tool := range r.tools() {
		if tool.Name == strings.TrimSpace(name) {
			return tool, nil
		}
	}
	return nil, fmt.Errorf("skills builtin: unknown tool %q", name)
}

func (r *Runner) ToolList(context.Context) ([]*mcp.Tool, error) {
	return r.tools(), nil
}

func (r *Runner) Invoke(_ context.Context, tool string, arguments map[string]any) (types.ToolResult, error) {
	switch strings.TrimSpace(tool) {
	case ToolGetRootDir:
		return types.ToolResult{Output: map[string]any{"skills_root_dir": r.RootDir()}}, nil
	case ToolListSkills:
		return types.ToolResult{Output: map[string]any{"skills": r.descriptors()}}, nil
	case ToolActivateSkill:
		return r.activate(arguments)
	case ToolReadResource:
		return r.readResource(arguments)
	default:
		err := fmt.Errorf("skills builtin: unknown tool %q", tool)
		return types.ToolResult{Err: err}, err
	}
}

func (r *Runner) Assess(tool string, _ map[string]any) types.ToolPolicy {
	switch strings.TrimSpace(tool) {
	case ToolActivateSkill, ToolReadResource:
		return types.ToolPolicy{
			Capabilities: []string{"workspace_read", "instructions"},
			Risk:         "low",
			Summary:      "loads workspace-owned Agent Skill instructions",
			OutputTrust:  types.TrustWorkspaceInstruction,
		}
	default:
		return types.ToolPolicy{
			Capabilities: []string{"runtime_metadata"},
			Risk:         "low",
			Summary:      "reads Agent Skills catalog metadata",
			OutputTrust:  types.TrustRuntimeMetadata,
		}
	}
}

func (r *Runner) tools() []*mcp.Tool {
	names := r.catalog.Names()
	return []*mcp.Tool{
		{
			Name:        ToolGetRootDir,
			Description: "Return the workspace Agent Skills root directory.",
			InputSchema: emptyObjectSchema(),
		},
		{
			Name:        ToolListSkills,
			Description: "List available Agent Skills and their supporting resource files.",
			InputSchema: emptyObjectSchema(),
		},
		{
			Name:        ToolActivateSkill,
			Description: "Activate one exact Agent Skill. Its complete SKILL.md instructions remain active for the conversation.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name": map[string]any{
						"type":        "string",
						"enum":        names,
						"description": "Exact skill name from AVAILABLE SKILLS.",
					},
				},
				"required":             []string{"name"},
				"additionalProperties": false,
			},
		},
		{
			Name:        ToolReadResource,
			Description: "Read a supporting UTF-8 resource from an activated Agent Skill. Use only paths listed for that skill.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"skill": map[string]any{"type": "string", "enum": names},
					"path": map[string]any{
						"type":        "string",
						"description": "Resource path relative to the skill directory, as listed in AVAILABLE SKILLS.",
					},
					"offset_runes": map[string]any{"type": "integer", "minimum": 0},
					"limit_runes":  map[string]any{"type": "integer", "minimum": 1, "maximum": skills.MaxResourceRunes},
				},
				"required":             []string{"skill", "path"},
				"additionalProperties": false,
			},
		},
	}
}

func (r *Runner) activate(arguments map[string]any) (types.ToolResult, error) {
	name, _ := arguments["name"].(string)
	skill, ok := r.catalog.Get(strings.TrimSpace(name))
	if !ok {
		err := fmt.Errorf("skills builtin: no skill named %q", strings.TrimSpace(name))
		return types.ToolResult{Err: err}, err
	}
	resources := make([]string, 0, len(skill.Resources))
	for _, resource := range skill.Resources {
		resources = append(resources, resource.Path)
	}
	return types.ToolResult{
		Output: map[string]any{
			"activated":   skill.Name,
			"description": skill.Description,
			"version":     skill.Version(),
			"digest":      skill.Digest,
			"resources":   resources,
		},
		Artifacts: []types.ContextArtifact{{
			Kind:    types.ArtifactAgentSkill,
			Name:    skill.Name,
			Content: skill.Instructions,
			Source:  skill.SourcePath,
			Digest:  skill.Digest,
			Trust:   types.TrustWorkspaceInstruction,
		}},
	}, nil
}

func (r *Runner) readResource(arguments map[string]any) (types.ToolResult, error) {
	skillName, _ := arguments["skill"].(string)
	path, _ := arguments["path"].(string)
	offset, err := integerArgument(arguments, "offset_runes", 0)
	if err != nil {
		return types.ToolResult{Err: err}, err
	}
	limit, err := integerArgument(arguments, "limit_runes", skills.DefaultResourceRunes)
	if err != nil {
		return types.ToolResult{Err: err}, err
	}
	page, err := r.catalog.ReadResource(skillName, path, offset, limit)
	if err != nil {
		return types.ToolResult{Err: err}, err
	}
	return types.ToolResult{Output: map[string]any{
		"skill":             page.Skill,
		"path":              page.Path,
		"text":              page.Text,
		"offset_runes":      page.OffsetRunes,
		"limit_runes":       page.LimitRunes,
		"total_runes":       page.TotalRunes,
		"next_offset_runes": page.NextOffsetRunes,
		"eof":               page.EOF,
	}}, nil
}

func (r *Runner) descriptors() []map[string]any {
	all := r.AllSkills()
	out := make([]map[string]any, 0, len(all))
	for _, skill := range all {
		resources := make([]string, 0, len(skill.Resources))
		for _, resource := range skill.Resources {
			resources = append(resources, resource.Path)
		}
		out = append(out, map[string]any{
			"name":          skill.Name,
			"description":   skill.Description,
			"source_file":   skill.SourceFile,
			"version":       skill.Version(),
			"resources":     resources,
			"allowed_tools": skill.AllowedTools,
			"byte_size":     skill.ByteSize,
		})
	}
	return out
}

func emptyObjectSchema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false}
}

func integerArgument(arguments map[string]any, key string, fallback int) (int, error) {
	value, ok := arguments[key]
	if !ok || value == nil {
		return fallback, nil
	}
	switch n := value.(type) {
	case int:
		return n, nil
	case int64:
		return int(n), nil
	case float64:
		if n != float64(int(n)) {
			return 0, fmt.Errorf("skills builtin: %s must be an integer", key)
		}
		return int(n), nil
	default:
		return 0, fmt.Errorf("skills builtin: %s must be an integer", key)
	}
}
