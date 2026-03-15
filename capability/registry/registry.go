package registry

import (
	"sort"

	"github.com/OctoSucker/octosucker-utils/graph"
)

const EntryNodeName = "start"

type Registry struct {
	g *graph.Graph
}

func NewRegistry() *Registry {
	r := &Registry{g: graph.NewGraph()}
	r.RegisterBuiltin()
	r.RegisterEntry()
	return r
}

func (r *Registry) Register(node *graph.Node) {
	r.g.AddNode(node)
}

func (r *Registry) Get(name string) (*graph.Node, bool) {
	n := r.g.GetNode(name)
	return n, n != nil
}

func (r *Registry) GetByTool(toolName string) (*graph.Node, bool) {
	if toolName == "" {
		return nil, false
	}
	for _, n := range r.g.Nodes {
		if n != nil && n.Tool == toolName {
			return n, true
		}
	}
	return nil, false
}

func (r *Registry) List() []*graph.Node {
	out := make([]*graph.Node, 0, len(r.g.Nodes))
	for _, n := range r.g.Nodes {
		out = append(out, n)
	}
	return out
}

func (r *Registry) CopyNodes() map[string]*graph.Node {
	m := make(map[string]*graph.Node, len(r.g.Nodes))
	for k, v := range r.g.Nodes {
		m[k] = v
	}
	return m
}

func (r *Registry) RegisterBuiltin() {
	r.Register(&graph.Node{
		Name:        "finish",
		Description: "结束当前任务，向用户返回最终回答。当已收集足够信息或完成用户请求时调用。",
		InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}},
		Tool:        "",
		Next:        []string{},
	})
}

func (r *Registry) RegisterEntry() {
	r.Register(&graph.Node{
		Name:        EntryNodeName,
		Description: "任务入口，不可直接调用",
		InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}},
		Tool:        "",
		Next:        []string{},
	})
}

func (r *Registry) RegisterToolCapabilities(toolDefs []map[string]interface{}) {
	if r == nil || len(toolDefs) == 0 {
		return
	}
	for _, def := range toolDefs {
		fn, _ := def["function"].(map[string]interface{})
		if fn == nil {
			continue
		}
		name, _ := fn["name"].(string)
		if name == "" || name == EntryNodeName || name == "finish" {
			continue
		}
		description, _ := fn["description"].(string)
		params, _ := fn["parameters"].(map[string]interface{})
		if params == nil {
			params = map[string]interface{}{"type": "object", "properties": map[string]interface{}{}}
		}

		existing, ok := r.Get(name)
		if ok && existing != nil {
			if existing.Description == "" {
				existing.Description = description
			}
			if existing.InputSchema == nil {
				existing.InputSchema = params
			}
			if existing.Tool == "" {
				existing.Tool = name
			}
			if len(existing.Next) == 0 {
				existing.Next = []string{"finish"}
			}
			continue
		}

		r.Register(&graph.Node{
			Name:        name,
			Description: description,
			InputSchema: params,
			Tool:        name,
			Next:        []string{"finish"},
		})
	}
}

func (r *Registry) RefreshEntryCapabilities() {
	if r == nil {
		return
	}
	entry, ok := r.Get(EntryNodeName)
	if !ok || entry == nil {
		r.RegisterEntry()
		entry, _ = r.Get(EntryNodeName)
	}
	if entry == nil {
		return
	}

	nodes := r.List()
	names := make([]string, 0, len(nodes))
	for _, n := range nodes {
		if n == nil || n.Name == "" || n.Name == EntryNodeName {
			continue
		}
		names = append(names, n.Name)
	}
	sort.Strings(names)
	hasFinish := false
	for _, name := range names {
		if name == "finish" {
			hasFinish = true
			break
		}
	}
	if !hasFinish {
		names = append(names, "finish")
	}
	entry.Next = names
}

func (r *Registry) EntryCapabilities() []string {
	n, ok := r.Get(EntryNodeName)
	if !ok || n == nil {
		return []string{}
	}
	return n.Next
}
