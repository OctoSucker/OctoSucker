package workflow

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const skillsDir = "workspace/skills"
const skillFileName = "SKILL.md"

type skillFrontmatter struct {
	Name        string                 `yaml:"name"`
	Description string                 `yaml:"description"`
	Steps       []skillStepFrontmatter `yaml:"steps"`
}

type skillStepFrontmatter struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	Keywords    []string `yaml:"keywords"`
	Tool        string   `yaml:"tool"`
}

func LoadWorkflowTemplatesFromDir() ([]WorkflowTemplate, error) {
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []WorkflowTemplate
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		skillPath := filepath.Join(skillsDir, e.Name(), skillFileName)
		data, err := os.ReadFile(skillPath)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		content := bytes.TrimSpace(data)
		var yamlBlock []byte
		if bytes.HasPrefix(content, []byte("---")) {
			content = content[3:]
			if idx := bytes.Index(content, []byte("\n---")); idx >= 0 {
				yamlBlock = bytes.TrimSpace(content[:idx])
				content = bytes.TrimSpace(content[idx+4:])
			}
		}
		if len(yamlBlock) == 0 {
			continue
		}
		var fm skillFrontmatter
		if err := yaml.Unmarshal(yamlBlock, &fm); err != nil {
			continue
		}
		tpl := WorkflowTemplate{
			Name:        strings.TrimSpace(fm.Name),
			Description: strings.TrimSpace(fm.Description),
		}
		if tpl.Description == "" && len(content) > 0 {
			line, _, _ := bytes.Cut(content, []byte("\n"))
			tpl.Description = strings.TrimSpace(string(line))
		}
		if len(fm.Steps) > 0 {
			tpl.Steps = make([]WorkflowStepTemplate, 0, len(fm.Steps))
			for _, s := range fm.Steps {
				tpl.Steps = append(tpl.Steps, WorkflowStepTemplate{
					Name:        strings.TrimSpace(s.Name),
					Description: strings.TrimSpace(s.Description),
					Keywords:    s.Keywords,
					Tool:        strings.TrimSpace(s.Tool),
				})
			}
		} else {
			stepName := tpl.Name
			if stepName == "" {
				stepName = "run"
			}
			tpl.Steps = []WorkflowStepTemplate{{
				Name:        stepName,
				Description: tpl.Description,
				Keywords:    nil,
				Tool:        "",
			}}
		}
		if strings.TrimSpace(tpl.Name) == "" {
			tpl.Name = e.Name()
		}
		if len(tpl.Steps) == 0 {
			continue
		}
		out = append(out, tpl)
	}
	return out, nil
}
