package registry

type Plugin struct {
	ID          string   `json:"id"`
	DisplayName string   `json:"display_name,omitempty"`
	Command     []string `json:"command,omitempty"`
	Endpoint    string   `json:"endpoint,omitempty"`
}

type Registry struct {
	plugins []Plugin
}

func New() *Registry {
	return &Registry{}
}

func (r *Registry) Register(p Plugin) {
	if p.ID == "" {
		panic("mcpsvc/registry: plugin id required")
	}
	r.plugins = append(r.plugins, p)
}

func (r *Registry) All() []Plugin {
	return append([]Plugin(nil), r.plugins...)
}
