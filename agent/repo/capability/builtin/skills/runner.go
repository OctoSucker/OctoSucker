package skillsbuiltin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/OctoSucker/agent/model"
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
	dir    string
	db     *model.AgentDB
	byName map[string]Skill
	llm    *llmclient.OpenAI
}

// NewRunner loads skills from workspace SQLite, merges markdown under dir (new files parsed and upserted), and returns the skills capability runner.
func NewRunner(ctx context.Context, db *model.AgentDB, dir string, llm *llmclient.OpenAI) (*Runner, error) {
	if db == nil {
		return nil, fmt.Errorf("skills builtin: agent db is required")
	}
	if dir == "" {
		return nil, fmt.Errorf("skills builtin: directory is required")
	}
	r := &Runner{
		dir:    dir,
		db:     db,
		byName: make(map[string]Skill),
		llm:    llm,
	}
	if err := r.Reload(ctx, llm); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *Runner) Name() string { return CapabilityName }

func (r *Runner) RootDir() string {
	if r == nil {
		return ""
	}
	return r.dir
}

func (r *Runner) Reload(ctx context.Context, llm *llmclient.OpenAI) error {
	if r == nil {
		return fmt.Errorf("skills builtin: runner is nil")
	}
	if r.db == nil {
		return fmt.Errorf("skills builtin: agent db is required")
	}
	if r.dir == "" {
		return fmt.Errorf("skills builtin: directory is required")
	}
	byName, err := r.syncFromDir(ctx, llm)
	if err != nil {
		return err
	}
	r.byName = byName
	return nil
}

// syncFromDir loads every skill row from SQLite into memory, then scans skills root for *.md.
// For each markdown file whose source_file is already stored, the cached JSON payload is used (no LLM).
// For a new file (basename e.g. foo.md with no row for that source_file), the file is parsed with the LLM;
// the parsed skill name must equal the file stem (foo). New or updated rows are written with SkillUpsert.
func (r *Runner) syncFromDir(ctx context.Context, llm *llmclient.OpenAI) (map[string]Skill, error) {
	rows, err := r.db.SkillsSelectAllRows()
	if err != nil {
		return nil, fmt.Errorf("skills builtin: load skills from sqlite: %w", err)
	}

	byName := make(map[string]Skill, len(rows))
	bySource := make(map[string]Skill, len(rows))
	for _, row := range rows {
		sk, derr := decodeSkillJSON([]byte(row.Payload))
		if derr != nil {
			return nil, fmt.Errorf("skills builtin: decode stored skill %q: %w", row.Name, derr)
		}
		if verr := validateSkill(sk); verr != nil {
			return nil, fmt.Errorf("skills builtin: stored skill %q: %w", row.Name, verr)
		}
		fullPath := filepath.Join(r.dir, filepath.FromSlash(row.SourceFile))
		sk.SourcePath = fullPath
		byName[sk.Name] = sk
		bySource[row.SourceFile] = sk
	}

	entries, err := os.ReadDir(r.dir)
	if err != nil {
		return nil, fmt.Errorf("skills builtin: read dir %q: %w", r.dir, err)
	}

	mdFiles := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.EqualFold(filepath.Ext(entry.Name()), ".md") {
			mdFiles = append(mdFiles, filepath.Join(r.dir, entry.Name()))
		}
	}
	sort.Strings(mdFiles)

	for _, fullPath := range mdFiles {
		base := filepath.Base(fullPath)
		rel := filepath.ToSlash(base)
		stem := strings.TrimSuffix(base, filepath.Ext(base))

		if existing, ok := bySource[rel]; ok {
			existing.SourcePath = fullPath
			byName[existing.Name] = existing
			bySource[rel] = existing
			continue
		}

		if llm == nil {
			return nil, fmt.Errorf("skills builtin: llm is required to parse new skill markdown %q", fullPath)
		}
		raw, rerr := os.ReadFile(fullPath)
		if rerr != nil {
			return nil, fmt.Errorf("skills builtin: read skill file %q: %w", fullPath, rerr)
		}
		sk, perr := parseSkillMarkdown(ctx, llm, fullPath, string(raw))
		if perr != nil {
			return nil, fmt.Errorf("skills builtin: parse skill file %q: %w", fullPath, perr)
		}
		if sk.Name != stem {
			return nil, fmt.Errorf("skills builtin: skill name %q must match markdown basename %q", sk.Name, stem)
		}
		payload, merr := json.Marshal(sk)
		if merr != nil {
			return nil, fmt.Errorf("skills builtin: marshal skill %q: %w", sk.Name, merr)
		}
		if uerr := r.db.SkillUpsert(model.SkillPersistRow{
			Name:       sk.Name,
			SourceFile: rel,
			Payload:    string(payload),
		}); uerr != nil {
			return nil, fmt.Errorf("skills builtin: sqlite upsert %q: %w", sk.Name, uerr)
		}
		bySource[rel] = sk
		byName[sk.Name] = sk
	}

	return byName, nil
}

func (r *Runner) allSkills() []Skill {
	if r == nil {
		return nil
	}
	names := make([]string, 0, len(r.byName))
	for k := range r.byName {
		names = append(names, k)
	}
	sort.Strings(names)
	out := make([]Skill, 0, len(names))
	for _, name := range names {
		out = append(out, r.byName[name])
	}
	return out
}

func (r *Runner) getSkill(name string) (Skill, bool) {
	if r == nil {
		return Skill{}, false
	}
	sk, ok := r.byName[name]
	return sk, ok
}

// PlannerBundle returns structured skill docs for planner prompts and MCP tools.
func (r *Runner) PlannerBundle() PromptBundle {
	if r == nil {
		return PromptBundle{}
	}
	return PromptBundle{RootDir: r.dir, Skills: r.allSkills()}
}

func (r *Runner) plannerAppendix() string {
	return FormatPromptAppendix(r.PlannerBundle())
}

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

// BoundMcpTools returns planner-invokable tools derived from loaded skills (skillslug__toolslug).
// They are not included in ToolList and must be listed separately in the planner appendix.
func (r *Runner) BoundMcpTools() ([]*mcp.Tool, error) {
	bound := r.boundSkillTools()
	out := make([]*mcp.Tool, 0, len(bound))
	for _, e := range bound {
		t, err := r.Tool(e.MCPName)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, nil
}

func (r *Runner) boundSkillTools() []boundSkillTool {
	all := r.allSkills()
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
			Description: "Get local skills root directory (markdown sources scanned on reload; canonical copy is workspace SQLite)",
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
			Description: "Reload from workspace SQLite, rescan *.md under skills root; new markdown files are parsed and upserted (cached rows reused when source_file matches)",
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
	return r.builtinTools(), nil
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
				"skills_root_dir": r.RootDir(),
			},
		}, nil
	case ToolListSkills:
		all := r.allSkills()
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
		sk, ok := r.getSkill(strings.TrimSpace(rawName))
		if !ok {
			return ports.ToolResult{}, fmt.Errorf("skills builtin: no skill named %q", rawName)
		}
		return ports.ToolResult{OK: true, Output: sk}, nil
	case ToolGetPlannerAppendix:
		return ports.ToolResult{
			OK:     true,
			Output: map[string]any{"appendix": r.plannerAppendix()},
		}, nil
	case ToolReloadSkills:
		if err := r.Reload(ctx, r.llm); err != nil {
			return ports.ToolResult{}, err
		}
		all := r.allSkills()
		names := make([]string, 0, len(all))
		for _, sk := range all {
			names = append(names, sk.Name)
		}
		return ports.ToolResult{
			OK: true,
			Output: map[string]any{
				"skills_root_dir": r.RootDir(),
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
