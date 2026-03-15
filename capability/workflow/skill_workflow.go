package workflow

import (
	"strings"

	"github.com/OctoSucker/octosucker-utils/graph"
	"github.com/OctoSucker/octosucker/capability/registry"
)

type WorkflowTemplate struct {
	Name        string
	Description string
	Steps       []WorkflowStepTemplate
}

type WorkflowStepTemplate struct {
	Name        string
	Description string
	Keywords    []string
	Tool        string
}

func RegisterSkillWorkflows(reg *registry.Registry, templates []WorkflowTemplate) {
	if reg == nil {
		return
	}
	sanitize := func(s string) string {
		s = strings.ToLower(strings.TrimSpace(s))
		if s == "" {
			return "step"
		}
		var b strings.Builder
		lastUnderscore := false
		for _, r := range s {
			isAlphaNum := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
			if isAlphaNum {
				b.WriteRune(r)
				lastUnderscore = false
				continue
			}
			if !lastUnderscore {
				b.WriteRune('_')
				lastUnderscore = true
			}
		}
		out := strings.Trim(b.String(), "_")
		if out == "" {
			return "step"
		}
		return out
	}

	for _, tpl := range templates {
		if strings.TrimSpace(tpl.Name) == "" || len(tpl.Steps) == 0 {
			continue
		}
		startName := "skill_" + sanitize(tpl.Name) + "_start"
		used := make(map[string]bool)
		stages := make([]*graph.Node, 0, len(tpl.Steps))
		for _, step := range tpl.Steps {
			var stage *graph.Node
			toolName := strings.TrimSpace(step.Tool)
			if toolName != "" {
				for _, node := range reg.List() {
					if node == nil || node.Name == "" || node.Tool == "" || used[node.Name] {
						continue
					}
					if node.Name == registry.EntryNodeName || node.Name == "finish" {
						continue
					}
					if node.Tool == toolName {
						used[node.Name] = true
						stage = node
						break
					}
				}
			}
			if stage == nil {
				bestScore := -1
				var best *graph.Node
				for _, node := range reg.List() {
					if node == nil || node.Name == "" || node.Tool == "" || used[node.Name] {
						continue
					}
					if node.Name == registry.EntryNodeName || node.Name == "finish" {
						continue
					}
					text := strings.ToLower(node.Name + " " + node.Description + " " + node.Tool)
					score := 0
					for _, kw := range step.Keywords {
						if strings.Contains(text, kw) {
							score++
						}
					}
					if score > bestScore {
						bestScore = score
						best = node
					}
				}
				if best != nil && bestScore > 0 {
					used[best.Name] = true
					stage = best
				}
			}
			if stage == nil {
				break
			}
			stages = append(stages, stage)
		}
		if len(stages) != len(tpl.Steps) {
			continue
		}

		reg.Register(&graph.Node{
			Name:        startName,
			Description: "技能编排：" + strings.TrimSpace(tpl.Description),
			InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}},
			Tool:        "",
			Next:        []string{"skill_" + sanitize(tpl.Name) + "_" + sanitize(tpl.Steps[0].Name)},
		})

		for i, stage := range stages {
			step := tpl.Steps[i]
			stepNodeName := "skill_" + sanitize(tpl.Name) + "_" + sanitize(step.Name)
			next := []string{"finish"}
			if i+1 < len(tpl.Steps) {
				next = []string{"skill_" + sanitize(tpl.Name) + "_" + sanitize(tpl.Steps[i+1].Name)}
			}
			schema := stage.InputSchema
			if schema == nil {
				schema = map[string]interface{}{"type": "object", "properties": map[string]interface{}{}}
			}
			reg.Register(&graph.Node{
				Name:        stepNodeName,
				Description: "流程步骤：" + strings.TrimSpace(step.Description),
				InputSchema: schema,
				Tool:        stage.Tool,
				Next:        next,
			})
		}
	}
}
