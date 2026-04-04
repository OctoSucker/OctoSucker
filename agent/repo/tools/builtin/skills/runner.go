package skillsbuiltin

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/OctoSucker/agent/pkg/ports"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	ToolGetRootDir         = "get_skills_root_dir"
	ToolReloadSkills       = "reload_skills"
	ToolListSkills         = "list_skills"
	ToolReadSkill          = "read_skill"
	// ToolListSkillNamesForPlanner returns a compact name/path index for the planner (not full skill bodies).
	ToolListSkillNamesForPlanner = "list_skill_names_for_planner"

	defaultReadLimitRunes = 8000
	maxReadLimitRunes     = 50000
)

type Runner struct {
	dir    string
	byName map[string]SkillMeta
}

// NewRunner scans dir for *.md skill files and returns the skills tool backend.
func NewRunner(dir string) (*Runner, error) {
	if dir == "" {
		return nil, fmt.Errorf("skills builtin: directory is required")
	}
	r := &Runner{dir: dir}
	if err := r.Reload(); err != nil {
		return nil, err
	}
	return r, nil
}

// Name is the ToolRegistry.Backends map key for this provider (not a user-facing tool id).
func (r *Runner) Name() string { return "skills" }

func (r *Runner) RootDir() string {
	if r == nil {
		return ""
	}
	return r.dir
}

func (r *Runner) Reload() error {
	if r == nil {
		return fmt.Errorf("skills builtin: backend is nil")
	}
	if r.dir == "" {
		return fmt.Errorf("skills builtin: directory is required")
	}
	byName, err := scanSkillDir(r.dir)
	if err != nil {
		return err
	}
	r.byName = byName
	return nil
}

func scanSkillDir(dir string) (map[string]SkillMeta, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("skills builtin: read dir %q: %w", dir, err)
	}
	byName := make(map[string]SkillMeta)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.EqualFold(filepath.Ext(entry.Name()), ".md") {
			continue
		}
		base := filepath.Base(entry.Name())
		stem := strings.TrimSuffix(base, filepath.Ext(base))
		fullPath := filepath.Join(dir, base)
		st, statErr := os.Stat(fullPath)
		if statErr != nil {
			return nil, fmt.Errorf("skills builtin: stat %q: %w", fullPath, statErr)
		}
		byName[stem] = SkillMeta{
			Name:       stem,
			SourceFile: filepath.ToSlash(base),
			SourcePath: fullPath,
			ByteSize:   st.Size(),
		}
	}
	return byName, nil
}

func (r *Runner) allSkills() []SkillMeta {
	if r == nil {
		return nil
	}
	names := make([]string, 0, len(r.byName))
	for k := range r.byName {
		names = append(names, k)
	}
	sort.Strings(names)
	out := make([]SkillMeta, 0, len(names))
	for _, name := range names {
		out = append(out, r.byName[name])
	}
	return out
}

func (r *Runner) getSkillMeta(name string) (SkillMeta, bool) {
	if r == nil {
		return SkillMeta{}, false
	}
	sk, ok := r.byName[name]
	return sk, ok
}

func (r *Runner) PlannerBundle() PromptBundle {
	if r == nil {
		return PromptBundle{}
	}
	return PromptBundle{RootDir: r.dir, Skills: r.allSkills()}
}

func (r *Runner) plannerAppendix() string {
	bundle := r.PlannerBundle()
	return bundle.FormatPromptAppendix()
}

func emptyObjectSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"properties":           map[string]any{},
		"additionalProperties": false,
	}
}

func readSkillInputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{
				"type":        "string",
				"description": "Skill name (markdown stem) as returned by list_skills",
			},
			"offset_runes": map[string]any{
				"type":        "integer",
				"description": "0-based rune offset into the file; omit or 0 for start. Use next_offset_runes from the previous read_skill until eof.",
				"minimum":     0,
			},
			"limit_runes": map[string]any{
				"type":        "integer",
				"description": fmt.Sprintf("Max runes to return (default %d, max %d)", defaultReadLimitRunes, maxReadLimitRunes),
				"minimum":     1,
				"maximum":     maxReadLimitRunes,
			},
		},
		"required":             []string{"name"},
		"additionalProperties": false,
	}
}

func (r *Runner) builtinTools() []*mcp.Tool {
	return []*mcp.Tool{
		{
			Name:        ToolGetRootDir,
			Description: "Get local skills root directory (top-level *.md files are skills)",
			InputSchema: emptyObjectSchema(),
		},
		{
			Name:        ToolListSkills,
			Description: "List markdown skill files (name, path, size). Use read_skill to load content in pages.",
			InputSchema: emptyObjectSchema(),
		},
		{
			Name:        ToolReadSkill,
			Description: "Read one skill markdown file as UTF-8 text; paginate with offset_runes / limit_runes using next_offset_runes until eof.",
			InputSchema: readSkillInputSchema(),
		},
		{
			Name:        ToolListSkillNamesForPlanner,
			Description: "Return skill names and source paths only (compact index for planning; use read_skill for full markdown)",
			InputSchema: emptyObjectSchema(),
		},
		{
			Name:        ToolReloadSkills,
			Description: "Rescan the skills root for *.md files (picks up adds/removes/renames)",
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
	return false
}

func (r *Runner) Tool(tool string) (*mcp.Tool, error) {
	for _, t := range r.builtinTools() {
		if t.Name == tool {
			return t, nil
		}
	}
	return nil, fmt.Errorf("skills builtin: unknown tool %q", tool)
}

func (r *Runner) ToolList(ctx context.Context) ([]*mcp.Tool, error) {
	return r.builtinTools(), nil
}

func (r *Runner) Invoke(ctx context.Context, localTool string, arguments map[string]any) (ports.ToolResult, error) {
	switch localTool {
	case ToolGetRootDir:
		return ports.ToolResult{
			Output: map[string]any{
				"skills_root_dir": r.RootDir(),
			},
		}, nil
	case ToolListSkills:
		all := r.allSkills()
		list := make([]map[string]any, 0, len(all))
		for _, sk := range all {
			list = append(list, map[string]any{
				"name":        sk.Name,
				"source_file": sk.SourceFile,
				"source_path": sk.SourcePath,
				"byte_size":   sk.ByteSize,
			})
		}
		return ports.ToolResult{Output: map[string]any{"skills": list}}, nil
	case ToolReadSkill:
		return r.invokeReadSkill(arguments)
	case ToolListSkillNamesForPlanner:
		return ports.ToolResult{
			Output: map[string]any{"appendix": r.plannerAppendix()},
		}, nil
	case ToolReloadSkills:
		if err := r.Reload(); err != nil {
			return ports.ToolResult{Err: err}, err
		}
		all := r.allSkills()
		names := make([]string, 0, len(all))
		for _, sk := range all {
			names = append(names, sk.Name)
		}
		return ports.ToolResult{
			Output: map[string]any{
				"skills_root_dir": r.RootDir(),
				"loaded_count":    len(all),
				"skills":          names,
			},
		}, nil
	default:
		return ports.ToolResult{}, fmt.Errorf("skills builtin: unknown tool %q", localTool)
	}
}

func (r *Runner) invokeReadSkill(args map[string]any) (ports.ToolResult, error) {
	if args == nil {
		return ports.ToolResult{Err: fmt.Errorf("skills builtin: read_skill requires arguments")}, fmt.Errorf("skills builtin: read_skill requires arguments")
	}
	rawName, ok := args["name"].(string)
	if !ok || strings.TrimSpace(rawName) == "" {
		return ports.ToolResult{Err: fmt.Errorf("skills builtin: read_skill argument \"name\" must be non-empty string")}, fmt.Errorf("skills builtin: read_skill argument \"name\" must be non-empty string")
	}
	name := strings.TrimSpace(rawName)
	meta, ok := r.getSkillMeta(name)
	if !ok {
		return ports.ToolResult{Err: fmt.Errorf("skills builtin: no skill named %q", name)}, fmt.Errorf("skills builtin: no skill named %q", name)
	}
	offset, err := intFromArgs(args, "offset_runes", 0)
	if err != nil {
		return ports.ToolResult{Err: fmt.Errorf("skills builtin: read_skill offset_runes: %w", err)}, fmt.Errorf("skills builtin: read_skill offset_runes: %w", err)
	}
	if offset < 0 {
		return ports.ToolResult{Err: fmt.Errorf("skills builtin: read_skill offset_runes must be >= 0")}, fmt.Errorf("skills builtin: read_skill offset_runes must be >= 0")
	}
	limit, err := intFromArgs(args, "limit_runes", defaultReadLimitRunes)
	if err != nil {
		return ports.ToolResult{Err: fmt.Errorf("skills builtin: read_skill limit_runes: %w", err)}, fmt.Errorf("skills builtin: read_skill limit_runes: %w", err)
	}
	if limit < 1 {
		limit = defaultReadLimitRunes
	}
	if limit > maxReadLimitRunes {
		limit = maxReadLimitRunes
	}

	raw, err := os.ReadFile(meta.SourcePath)
	if err != nil {
		return ports.ToolResult{Err: fmt.Errorf("skills builtin: read %q: %w", meta.SourcePath, err)}, fmt.Errorf("skills builtin: read %q: %w", meta.SourcePath, err)
	}
	if !utf8.Valid(raw) {
		return ports.ToolResult{Err: fmt.Errorf("skills builtin: %q is not valid UTF-8", meta.SourcePath)}, fmt.Errorf("skills builtin: %q is not valid UTF-8", meta.SourcePath)
	}
	rs := []rune(string(raw))
	total := len(rs)
	if offset > total {
		offset = total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	chunk := string(rs[offset:end])
	next := end
	eof := next >= total

	return ports.ToolResult{
		Output: map[string]any{
			"name":               meta.Name,
			"source_path":        meta.SourcePath,
			"source_file":        meta.SourceFile,
			"text":               chunk,
			"offset_runes":       offset,
			"limit_runes":        limit,
			"total_runes":        total,
			"next_offset_runes":  next,
			"eof":                eof,
			"returned_rune_span": end - offset,
		},
	}, nil
}

func intFromArgs(m map[string]any, key string, defaultVal int) (int, error) {
	v, ok := m[key]
	if !ok || v == nil {
		return defaultVal, nil
	}
	switch x := v.(type) {
	case float64:
		return int(x), nil
	case int:
		return x, nil
	case int64:
		return int(x), nil
	default:
		return 0, fmt.Errorf("%q must be a number", key)
	}
}
