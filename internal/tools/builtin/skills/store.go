package skillsbuiltin

// SkillMeta is one markdown skill file under the skills root.
type SkillMeta struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Summary     string         `json:"summary,omitempty"`
	SourceFile  string         `json:"source_file"`
	SourcePath  string         `json:"source_path"`
	ByteSize    int64          `json:"byte_size"`
	CLIPlugin   *CLIPluginSpec `json:"cli_plugin,omitempty"`
}

type CLIPluginSpec struct {
	Provider string            `yaml:"provider" json:"provider"`
	Command  string            `yaml:"command" json:"command"`
	WorkDir  string            `yaml:"work_dir,omitempty" json:"work_dir,omitempty"`
	Env      map[string]string `yaml:"env,omitempty" json:"env,omitempty"`
	Tools    []CLIPluginTool   `yaml:"tools" json:"tools"`
}

type CLIPluginTool struct {
	Name        string         `yaml:"name" json:"name"`
	Description string         `yaml:"description" json:"description"`
	InputSchema map[string]any `yaml:"input_schema" json:"input_schema"`
	Args        []string       `yaml:"args" json:"args"`
}
