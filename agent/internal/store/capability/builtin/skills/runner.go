package skillsbuiltin

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/OctoSucker/agent/pkg/llmclient"
	"github.com/OctoSucker/agent/pkg/ports"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	CapabilityName         = "skills"
	ToolGetRootDir         = "get_skills_root_dir"
	ToolReloadSkills       = "reload_skills"
	ToolListSkills         = "list_skills"
	ToolGetSkill           = "get_skill"
	ToolGetPlannerAppendix = "get_skills_planner_appendix"
)

type Runner struct {
	store *Store
	llm   *llmclient.OpenAI
}

func NewRunner(store *Store, llm *llmclient.OpenAI) (*Runner, error) {
	if store == nil {
		return nil, fmt.Errorf("skills builtin: store is required")
	}
	return &Runner{store: store, llm: llm}, nil
}

func (r *Runner) Name() string { return CapabilityName }

type boundSkillTool struct {
	MCPName    string
	Skill      Skill
	Capability string
	Tool       SkillTool
}

func mcpSlug(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "unnamed"
	}
	var b strings.Builder
	lastUnderscore := false
	for _, r := range s {
		ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_'
		if ok {
			b.WriteRune(r)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore {
			b.WriteByte('_')
			lastUnderscore = true
		}
	}
	out := strings.Trim(b.String(), "_")
	if out == "" {
		return "x"
	}
	return out
}

func (r *Runner) boundSkillTools() []boundSkillTool {
	all := r.store.All()
	var out []boundSkillTool
	seen := make(map[string]struct{})
	for _, sk := range all {
		for _, c := range sk.Capabilities {
			for _, t := range c.Tools {
				if strings.TrimSpace(t.Name) == "" {
					continue
				}
				base := mcpSlug(sk.Name) + "__" + mcpSlug(t.Name)
				var name string
				for suffix := 0; ; suffix++ {
					if suffix == 0 {
						name = base
					} else {
						name = fmt.Sprintf("%s__%d", base, suffix)
					}
					if _, dup := seen[name]; dup {
						continue
					}
					seen[name] = struct{}{}
					break
				}
				out = append(out, boundSkillTool{
					MCPName:    name,
					Skill:      sk,
					Capability: strings.TrimSpace(c.Capability),
					Tool:       t,
				})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].MCPName < out[j].MCPName })
	return out
}

func emptyObjectSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"properties":           map[string]any{},
		"additionalProperties": false,
	}
}

func inputSchemaForSkillTool(t SkillTool) (map[string]any, error) {
	if len(t.InputSchema) == 0 {
		return emptyObjectSchema(), nil
	}
	raw, err := json.Marshal(t.InputSchema)
	if err != nil {
		return nil, fmt.Errorf("skills builtin: marshal skill tool input_schema: %w", err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("skills builtin: skill tool input_schema: %w", err)
	}
	return m, nil
}

func (r *Runner) builtinTools() []*mcp.Tool {
	return []*mcp.Tool{
		{
			Name:        ToolGetRootDir,
			Description: "Get local skills root directory configured by skills_dir",
			InputSchema: emptyObjectSchema(),
		},
		{
			Name:        ToolListSkills,
			Description: "List loaded skills with capabilities and nested tools (name, description, usage, input_schema per tool)",
			InputSchema: emptyObjectSchema(),
		},
		{
			Name:        ToolGetSkill,
			Description: "Load one skill record by name (metadata and capabilities with nested tools)",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name": map[string]any{
						"type":        "string",
						"description": "Skill name as returned by list_skills",
					},
				},
				"required":             []string{"name"},
				"additionalProperties": false,
			},
		},
		{
			Name:        ToolGetPlannerAppendix,
			Description: "Return the full skills planner appendix text (markdown skill docs summary)",
			InputSchema: emptyObjectSchema(),
		},
		{
			Name:        ToolReloadSkills,
			Description: "Reload skills from markdown/json files in skills root directory",
			InputSchema: emptyObjectSchema(),
		},
	}
}

func (r *Runner) HasTool(name string) bool {
	if name == "" {
		return false
	}
	for _, t := range r.builtinTools() {
		if t.Name == name {
			return true
		}
	}
	for _, e := range r.boundSkillTools() {
		if e.MCPName == name {
			return true
		}
	}
	return false
}

func (r *Runner) Tool(tool string) (*mcp.Tool, error) {
	for _, t := range r.builtinTools() {
		if t.Name == tool {
			return t, nil
		}
	}
	for _, e := range r.boundSkillTools() {
		if e.MCPName != tool {
			continue
		}
		schema, err := inputSchemaForSkillTool(e.Tool)
		if err != nil {
			return nil, err
		}
		desc := strings.TrimSpace(e.Tool.Description)
		if desc == "" {
			desc = "Skill tool " + e.Tool.Name
		}
		desc = fmt.Sprintf("[%s] %s", e.Skill.Name, desc)
		if u := strings.TrimSpace(e.Tool.Usage); u != "" {
			desc += " | usage: " + u
		}
		return &mcp.Tool{
			Name:        e.MCPName,
			Description: desc,
			InputSchema: schema,
		}, nil
	}
	return nil, fmt.Errorf("skills builtin: unknown tool %q", tool)
}

func (r *Runner) ToolList(ctx context.Context) ([]*mcp.Tool, error) {
	out := append([]*mcp.Tool(nil), r.builtinTools()...)
	for _, e := range r.boundSkillTools() {
		t, err := r.Tool(e.MCPName)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, nil
}

func (r *Runner) findBoundTool(mcpName string) (boundSkillTool, bool) {
	for _, e := range r.boundSkillTools() {
		if e.MCPName == mcpName {
			return e, true
		}
	}
	return boundSkillTool{}, false
}

func (r *Runner) Invoke(ctx context.Context, inv ports.CapabilityInvocation) (ports.ToolResult, error) {
	switch inv.Tool {
	case ToolGetRootDir:
		return ports.ToolResult{
			OK: true,
			Output: map[string]any{
				"skills_root_dir": r.store.RootDir(),
			},
		}, nil
	case ToolListSkills:
		all := r.store.All()
		skills := make([]Skill, len(all))
		for i, sk := range all {
			skills[i] = sk
			if skills[i].Capabilities == nil {
				skills[i].Capabilities = []SkillCapability{}
			}
		}
		return ports.ToolResult{OK: true, Output: map[string]any{"skills": skills}}, nil
	case ToolGetSkill:
		if inv.Arguments == nil {
			return ports.ToolResult{}, fmt.Errorf("skills builtin: get_skill requires arguments")
		}
		rawName, ok := inv.Arguments["name"].(string)
		if !ok || strings.TrimSpace(rawName) == "" {
			return ports.ToolResult{}, fmt.Errorf("skills builtin: get_skill argument \"name\" must be non-empty string")
		}
		sk, ok := r.store.Get(strings.TrimSpace(rawName))
		if !ok {
			return ports.ToolResult{}, fmt.Errorf("skills builtin: no skill named %q", rawName)
		}
		return ports.ToolResult{OK: true, Output: sk}, nil
	case ToolGetPlannerAppendix:
		return ports.ToolResult{
			OK:     true,
			Output: map[string]any{"appendix": r.store.PlannerAppendix()},
		}, nil
	case ToolReloadSkills:
		if err := r.store.Reload(ctx, r.llm); err != nil {
			return ports.ToolResult{}, err
		}
		all := r.store.All()
		names := make([]string, 0, len(all))
		for _, sk := range all {
			names = append(names, sk.Name)
		}
		return ports.ToolResult{
			OK: true,
			Output: map[string]any{
				"skills_root_dir": r.store.RootDir(),
				"loaded_count":    len(all),
				"skills":          names,
			},
		}, nil
	default:
		if e, ok := r.findBoundTool(inv.Tool); ok {
			return ports.ToolResult{
				OK: true,
				Output: map[string]any{
					"skill":             e.Skill.Name,
					"skill_description": e.Skill.Description,
					"cautions":          e.Skill.Cautions,
					"source_path":       e.Skill.SourcePath,
					"capability":        e.Capability,
					"tool":              e.Tool.Name,
					"tool_description":  e.Tool.Description,
					"usage":             e.Tool.Usage,
					"arguments":         inv.Arguments,
				},
			}, nil
		}
		return ports.ToolResult{}, fmt.Errorf("skills builtin: unknown tool %q", inv.Tool)
	}
}
