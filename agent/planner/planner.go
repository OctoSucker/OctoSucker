package planner

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	capgraph "github.com/OctoSucker/octosucker-utils/graph"
	"github.com/OctoSucker/octosucker/agent/llm"
	capregistry "github.com/OctoSucker/octosucker/capability/registry"
)

const plannerSystemPrompt = `You are an AI planner. Break the user task into clear, actionable steps.
Reply with exactly one JSON object, no other text:
{"goal": "short goal phrase", "steps": ["step1", "step2", ...]}
Rules:
- Each step is one actionable subgoal in natural language. Do NOT use tool or API names.
- Use 3 to 6 steps. Fewer for simple tasks, more for complex ones.
- Steps should be ordered: first step first.`

const maxPlanSteps = 6

type Plan struct {
	Goal  string   `json:"goal"`
	Steps []string `json:"steps"`
}

type Planner struct {
	registry *capregistry.Registry
	llm      *llm.LLMClient
}

func NewPlannerWithLLM(registry *capregistry.Registry, llmClient *llm.LLMClient) *Planner {
	return &Planner{registry: registry, llm: llmClient}
}

func (p *Planner) Plan(ctx context.Context, query string) (*capgraph.Graph, error) {
	if p.llm == nil {
		return nil, nil
	}
	userPrompt := "User task:\n" + strings.TrimSpace(query)
	log.Printf("[planner] plannerSystemPrompt: %s", plannerSystemPrompt)
	log.Printf("[planner] user prompt: %s", userPrompt)
	content, err := p.llm.ChatCompletionWithSystem(ctx, plannerSystemPrompt, userPrompt)
	if err != nil {
		log.Printf("[planner] LLM call failed: %v", err)
		return nil, err
	}
	content = strings.TrimSpace(content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)
	var plan Plan
	if err := json.Unmarshal([]byte(content), &plan); err != nil {
		log.Printf("[planner] invalid JSON: %s", content)
		return nil, err
	}
	plan.Goal = strings.TrimSpace(plan.Goal)
	if len(plan.Steps) > maxPlanSteps {
		plan.Steps = plan.Steps[:maxPlanSteps]
	}
	for i := range plan.Steps {
		plan.Steps[i] = strings.TrimSpace(plan.Steps[i])
	}
	if len(plan.Steps) == 0 {
		return nil, nil
	}
	return p.BuildGraph(&plan), nil
}

func (p *Planner) EntryCapabilities() []string {
	return p.registry.EntryCapabilities()
}

func (p *Planner) BuildGraph(plan *Plan) *capgraph.Graph {
	if plan == nil || len(plan.Steps) == 0 {
		return nil
	}
	g := capgraph.NewGraph()
	names := make([]string, 0, len(plan.Steps))
	usedCap := make(map[string]int)
	for i, step := range plan.Steps {
		stepLower := strings.ToLower(strings.TrimSpace(step))
		stepWords := tokenize(stepLower)
		capName := ""
		if len(stepWords) > 0 {
			bestScore := 0
			for _, name := range p.registry.EntryCapabilities() {
				if name == capregistry.EntryNodeName {
					continue
				}
				node, ok := p.registry.Get(name)
				if !ok {
					continue
				}
				text := strings.ToLower(node.Name + " " + node.Description)
				score := 0
				for _, w := range stepWords {
					if len(w) >= 2 && (strings.Contains(text, w) || strings.Contains(stepLower, w)) {
						score++
					}
				}
				if score > bestScore {
					bestScore = score
					capName = name
				}
			}
		}
		if capName == "" {
			for _, name := range p.registry.EntryCapabilities() {
				if name != capregistry.EntryNodeName && name != "finish" {
					capName = name
					break
				}
			}
			if capName == "" {
				capName = "finish"
			}
		}
		template, ok := p.registry.Get(capName)
		if !ok {
			continue
		}
		usedCap[capName]++
		nodeName := capName
		if usedCap[capName] > 1 {
			nodeName = fmt.Sprintf("%s_%d", capName, i)
		}
		schema := template.InputSchema
		if schema != nil {
			copied := make(map[string]interface{}, len(schema))
			for k, v := range schema {
				copied[k] = v
			}
			schema = copied
		}
		g.AddNode(&capgraph.Node{
			Name:        nodeName,
			Description: template.Description,
			InputSchema: schema,
			Tool:        template.Tool,
			Next:        nil,
			Dynamic:     template.Dynamic,
		})
		names = append(names, nodeName)
	}
	for i := 1; i < len(names); i++ {
		g.AddEdge(names[i-1], names[i])
	}
	if finish, _ := p.registry.Get("finish"); finish != nil && len(names) > 0 {
		g.AddNode(finish)
		g.AddEdge(names[len(names)-1], "finish")
	}
	if len(names) > 0 {
		g.SetCurrent([]string{names[0]})
	}
	return g
}
