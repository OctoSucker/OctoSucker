package skillsbuiltin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/OctoSucker/agent/pkg/llmclient"
)

type SkillTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Usage       string         `json:"usage"`
	InputSchema map[string]any `json:"input_schema,omitempty"`
}

// SkillCapability groups tools that run under one agent capability (e.g. exec, skills).
type SkillCapability struct {
	Capability string      `json:"capability"`
	Tools      []SkillTool `json:"tools"`
}

type Skill struct {
	Name         string            `json:"name"`
	Description  string            `json:"description"`
	Cautions     string            `json:"cautions,omitempty"`
	SourcePath   string            `json:"source_path"`
	Capabilities []SkillCapability `json:"capabilities"`
}

// UnmarshalJSON accepts the current shape {"capabilities":[{capability,tools}]} or the legacy
// flat {"tools":[{capability,name,...}]} and normalizes to Capabilities.
func (s *Skill) UnmarshalJSON(data []byte) error {
	type rawTool struct {
		Capability  string         `json:"capability"`
		Name        string         `json:"name"`
		Description string         `json:"description"`
		Usage       string         `json:"usage"`
		InputSchema map[string]any `json:"input_schema"`
	}
	type raw struct {
		Name         string            `json:"name"`
		Description  string            `json:"description"`
		Cautions     string            `json:"cautions"`
		SourcePath   string            `json:"source_path"`
		Capabilities []SkillCapability `json:"capabilities"`
		Tools        []rawTool         `json:"tools"`
	}
	var aux raw
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	s.Name = aux.Name
	s.Description = aux.Description
	s.Cautions = aux.Cautions
	s.SourcePath = aux.SourcePath
	if len(aux.Capabilities) > 0 {
		s.Capabilities = aux.Capabilities
		return nil
	}
	if len(aux.Tools) == 0 {
		s.Capabilities = nil
		return nil
	}
	order := make([]string, 0)
	seen := make(map[string]struct{})
	byCap := make(map[string][]SkillTool)
	for _, lt := range aux.Tools {
		capName := strings.TrimSpace(lt.Capability)
		st := SkillTool{
			Name:        lt.Name,
			Description: lt.Description,
			Usage:       lt.Usage,
			InputSchema: lt.InputSchema,
		}
		if _, ok := seen[capName]; !ok {
			seen[capName] = struct{}{}
			order = append(order, capName)
		}
		byCap[capName] = append(byCap[capName], st)
	}
	s.Capabilities = make([]SkillCapability, 0, len(order))
	for _, capName := range order {
		s.Capabilities = append(s.Capabilities, SkillCapability{
			Capability: capName,
			Tools:      byCap[capName],
		})
	}
	return nil
}

type Store struct {
	dir    string
	byName map[string]Skill
}

func NewFromDir(ctx context.Context, dir string, llm *llmclient.OpenAI) (*Store, error) {
	if dir == "" {
		return nil, fmt.Errorf("skills builtin: directory is required")
	}
	s := &Store{
		dir:    dir,
		byName: make(map[string]Skill),
	}
	if err := s.Reload(ctx, llm); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) RootDir() string {
	if s == nil {
		return ""
	}
	return s.dir
}

func (s *Store) Reload(ctx context.Context, llm *llmclient.OpenAI) error {
	if s == nil {
		return fmt.Errorf("skills builtin: store is nil")
	}
	if s.dir == "" {
		return fmt.Errorf("skills builtin: directory is required")
	}
	loaded, err := loadSkillsFromDir(ctx, s.dir, llm)
	if err != nil {
		return err
	}
	s.byName = loaded
	return nil
}

func loadSkillsFromDir(ctx context.Context, dir string, llm *llmclient.OpenAI) (map[string]Skill, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("skills builtin: read dir %q: %w", dir, err)
	}

	mdFiles := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.EqualFold(filepath.Ext(entry.Name()), ".md") {
			mdFiles = append(mdFiles, filepath.Join(dir, entry.Name()))
		}
	}
	sort.Strings(mdFiles)

	loaded := make(map[string]Skill, len(mdFiles))
	for _, p := range mdFiles {
		jsonPath := skillJSONPath(p)
		sk, err := readSkillJSON(jsonPath)
		if err == nil {
			if sk.SourcePath == "" {
				sk.SourcePath = p
			}
			if needsCapabilityBackfill(sk) && llm != nil {
				raw, rerr := os.ReadFile(p)
				if rerr != nil {
					return nil, fmt.Errorf("skills builtin: read skill markdown for capability backfill %q: %w", p, rerr)
				}
				sk2, perr := parseSkillMarkdown(ctx, llm, p, string(raw))
				if perr != nil {
					return nil, fmt.Errorf("skills builtin: capability backfill for %q: %w", p, perr)
				}
				sk = sk2
				if err := writeSkillJSON(jsonPath, sk); err != nil {
					return nil, fmt.Errorf("skills builtin: write skill json after capability backfill %q: %w", jsonPath, err)
				}
			}
		} else if !os.IsNotExist(err) {
			return nil, fmt.Errorf("skills builtin: read skill json %q: %w", jsonPath, err)
		} else {
			if llm == nil {
				return nil, fmt.Errorf("skills builtin: llm is required to parse markdown for %q", p)
			}
			raw, err := os.ReadFile(p)
			if err != nil {
				return nil, fmt.Errorf("skills builtin: read skill file %q: %w", p, err)
			}
			sk, err = parseSkillMarkdown(ctx, llm, p, string(raw))
			if err != nil {
				return nil, fmt.Errorf("skills builtin: parse skill file %q: %w", p, err)
			}
			if err := writeSkillJSON(jsonPath, sk); err != nil {
				return nil, fmt.Errorf("skills builtin: write skill json %q: %w", jsonPath, err)
			}
		}

		if sk.Name == "" {
			return nil, fmt.Errorf("skills builtin: parsed empty skill name for %q", p)
		}
		loaded[sk.Name] = sk
	}
	return loaded, nil
}

func (s *Store) All() []Skill {
	if s == nil {
		return nil
	}
	names := make([]string, 0, len(s.byName))
	for k := range s.byName {
		names = append(names, k)
	}
	sort.Strings(names)
	out := make([]Skill, 0, len(names))
	for _, name := range names {
		out = append(out, s.byName[name])
	}
	return out
}

func (s *Store) Get(name string) (Skill, bool) {
	if s == nil {
		return Skill{}, false
	}
	sk, ok := s.byName[name]
	return sk, ok
}

func (s *Store) PlannerAppendix() string {
	if s == nil {
		return ""
	}
	all := s.All()
	if len(all) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("Skills (from local markdown docs):\n")
	if s.dir != "" {
		b.WriteString("Skills root directory: ")
		b.WriteString(s.dir)
		b.WriteByte('\n')
	}
	for _, sk := range all {
		b.WriteString("- skill: ")
		b.WriteString(sk.Name)
		if sk.Description != "" {
			b.WriteString(" | ")
			b.WriteString(sk.Description)
		}
		if sk.Cautions != "" {
			b.WriteString(" | cautions: ")
			b.WriteString(sk.Cautions)
		}
		b.WriteString(" | source: ")
		b.WriteString(sk.SourcePath)
		b.WriteByte('\n')
		for _, c := range sk.Capabilities {
			b.WriteString("  - capability: ")
			if strings.TrimSpace(c.Capability) != "" {
				b.WriteString(c.Capability)
			} else {
				b.WriteString("(unset)")
			}
			b.WriteByte('\n')
			for _, t := range c.Tools {
				b.WriteString("    - tool: ")
				b.WriteString(t.Name)
				if t.Description != "" {
					b.WriteString(" | ")
					b.WriteString(t.Description)
				}
				if t.Usage != "" {
					b.WriteString(" | usage: ")
					b.WriteString(t.Usage)
				}
				if t.InputSchema != nil {
					raw, err := json.Marshal(t.InputSchema)
					if err != nil {
						return ""
					}
					b.WriteString(" | input_schema: ")
					b.Write(raw)
				}
				b.WriteByte('\n')
			}
		}
	}
	return b.String()
}

func needsCapabilityBackfill(sk Skill) bool {
	for _, c := range sk.Capabilities {
		if strings.TrimSpace(c.Capability) == "" {
			return true
		}
	}
	return false
}

func skillJSONPath(mdPath string) string {
	ext := filepath.Ext(mdPath)
	base := strings.TrimSuffix(mdPath, ext)
	return base + ".json"
}

func readSkillJSON(path string) (Skill, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Skill{}, err
	}
	var sk Skill
	if err := json.Unmarshal(raw, &sk); err != nil {
		return Skill{}, fmt.Errorf("unmarshal: %w", err)
	}
	return sk, nil
}

func writeSkillJSON(path string, sk Skill) error {
	raw, err := json.MarshalIndent(sk, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	raw = append(raw, '\n')
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		return err
	}
	return nil
}
