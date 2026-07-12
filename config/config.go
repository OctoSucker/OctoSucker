// Package config loads workspace config.json (OpenAI, MCP, exec sandbox, HTTP,
// Telegram, OpenCLI, and Agent Skills settings).
// Paths are resolved relative to the workspace root passed to LoadWorkspace.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type Workspace struct {
	OpenAI      OpenAI   `json:"openai"`
	Context     Context  `json:"context"`
	MCPEndpoint []string `json:"mcp_endpoint"`
	Exec        Exec     `json:"exec"`
	HTTP        HTTP     `json:"http"`
	Telegram    Telegram `json:"telegram"`
	OpenCLI     OpenCLI  `json:"opencli"`
	SkillsDir   string   `json:"skills_dir"` // based on workspace root
	Thinker     Thinker  `json:"thinker"`
}

// Thinker configures the knowledge-only agent (Markdown corpus directory).
type Thinker struct {
	KnowledgeMDDir string `json:"knowledge_md_dir"`
}

type OpenAI struct {
	APIKey         string     `json:"api_key"`
	BaseURL        string     `json:"base_url"`
	Models         RoleModels `json:"models"`
	EmbeddingModel string     `json:"embedding_model"`
}

type RoleModels struct {
	Planner   Model `json:"planner"`
	Evaluator Model `json:"evaluator"`
	Responder Model `json:"responder"`
}

type Model struct {
	Name           string `json:"name"`
	EnableThinking *bool  `json:"enable_thinking,omitempty"`
}

type Context struct {
	PlannerInputTokens   int `json:"planner_input_tokens"`
	EvaluatorInputTokens int `json:"evaluator_input_tokens"`
	ResponderInputTokens int `json:"responder_input_tokens"`
}

// HTTP configures the optional admin web UI (chat + knowledge graph). Empty listen disables it.
type HTTP struct {
	Listen string `json:"listen"`
}

type Telegram struct {
	BotToken       string  `json:"bot_token"`
	DefaultChatID  int64   `json:"default_chat_id"`
	AllowedChatIDs []int64 `json:"allowed_chat_ids,omitempty"`
}

// OpenCLI configures the optional schema-generated OpenCLI tool provider.
// Commands is an allowlist keyed by OpenCLI site/adapter name.
type OpenCLI struct {
	Command           string              `json:"command"`
	Commands          map[string][]string `json:"commands"`
	CommandTimeoutSec int                 `json:"command_timeout_sec"`
}

// Exec sandbox backends (exec.backend JSON field).
const (
	ExecBackendDocker           = "docker"
	ExecBackendMacOSSandboxExec = "macos_sandbox_exec"
	ExecBackendHost             = "host"
)

type Exec struct {
	Backend               string   `json:"backend"`
	MacOSSandboxProfile   string   `json:"macos_sandbox_profile"`
	WorkspaceDirs         []string `json:"workspace_dirs"`
	CommandTimeoutSec     int      `json:"command_timeout_sec"`
	CommandBlacklist      []string `json:"command_blacklist"`
	ContainerRuntime      string   `json:"container_runtime"`
	ContainerImage        string   `json:"container_image"`
	ContainerName         string   `json:"container_name"`
	ContainerWorkspaceDir string   `json:"container_workspace_dir"`
	ContainerReadOnlyRoot bool     `json:"container_readonly_root"`
	ContainerUser         string   `json:"container_user"`
}

func LoadWorkspace(workspaceRoot string) (*Workspace, error) {
	p := ConfigFile(workspaceRoot)
	raw, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("missing %s: copy workspace/config.example.json (repo root) to %s and set openai + mcp.endpoint", p, p)
		}
		return nil, fmt.Errorf("read %s: %w", p, err)
	}
	var cfg Workspace
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", p, err)
	}
	if strings.TrimSpace(cfg.OpenAI.Models.Planner.Name) == "" ||
		strings.TrimSpace(cfg.OpenAI.Models.Evaluator.Name) == "" ||
		strings.TrimSpace(cfg.OpenAI.Models.Responder.Name) == "" {
		return nil, fmt.Errorf("parse %s: openai.models planner, evaluator, and responder names are required", p)
	}
	if cfg.Context.PlannerInputTokens <= 0 {
		cfg.Context.PlannerInputTokens = 64000
	}
	if cfg.Context.EvaluatorInputTokens <= 0 {
		cfg.Context.EvaluatorInputTokens = 48000
	}
	if cfg.Context.ResponderInputTokens <= 0 {
		cfg.Context.ResponderInputTokens = 64000
	}
	if len(cfg.Exec.WorkspaceDirs) == 0 {
		cfg.Exec.WorkspaceDirs = []string{workspaceRoot}
	} else {
		resolved := make([]string, 0, len(cfg.Exec.WorkspaceDirs))
		for _, entry := range cfg.Exec.WorkspaceDirs {
			if entry == "" {
				return nil, fmt.Errorf("parse %s: exec.workspace_dirs contains empty path", p)
			}
			if filepath.IsAbs(entry) {
				resolved = append(resolved, filepath.Clean(entry))
				continue
			}
			resolved = append(resolved, filepath.Clean(filepath.Join(workspaceRoot, entry)))
		}
		cfg.Exec.WorkspaceDirs = resolved
	}
	if cfg.Exec.CommandTimeoutSec <= 0 {
		cfg.Exec.CommandTimeoutSec = 30
	}
	switch strings.TrimSpace(cfg.Exec.Backend) {
	case "":
		cfg.Exec.Backend = ExecBackendDocker
	case ExecBackendDocker, ExecBackendMacOSSandboxExec, ExecBackendHost:
	default:
		return nil, fmt.Errorf("parse %s: exec.backend must be %q, %q, or %q", p, ExecBackendDocker, ExecBackendMacOSSandboxExec, ExecBackendHost)
	}
	if cfg.Exec.Backend == ExecBackendMacOSSandboxExec {
		prof := strings.TrimSpace(cfg.Exec.MacOSSandboxProfile)
		if prof == "" {
			return nil, fmt.Errorf("parse %s: exec.macos_sandbox_profile is required when exec.backend is %q", p, ExecBackendMacOSSandboxExec)
		}
		if !filepath.IsAbs(prof) {
			prof = filepath.Clean(filepath.Join(workspaceRoot, prof))
		} else {
			prof = filepath.Clean(prof)
		}
		st, err := os.Stat(prof)
		if err != nil {
			return nil, fmt.Errorf("parse %s: exec.macos_sandbox_profile %q: %w", p, prof, err)
		}
		if st.IsDir() {
			return nil, fmt.Errorf("parse %s: exec.macos_sandbox_profile %q must be a file", p, prof)
		}
		cfg.Exec.MacOSSandboxProfile = prof
	}
	if cfg.Exec.ContainerRuntime == "" {
		cfg.Exec.ContainerRuntime = "docker"
	}
	if cfg.Exec.ContainerImage == "" {
		cfg.Exec.ContainerImage = "octosucker-exec-sandbox:latest"
	}
	if cfg.Exec.ContainerName == "" {
		cfg.Exec.ContainerName = "octosucker-agent-sandbox"
	}
	if cfg.Exec.ContainerWorkspaceDir == "" {
		cfg.Exec.ContainerWorkspaceDir = "/workspace"
	} else {
		containerPath := filepath.ToSlash(filepath.Clean(cfg.Exec.ContainerWorkspaceDir))
		if !strings.HasPrefix(containerPath, "/") {
			containerPath = filepath.ToSlash(filepath.Join("/workspace", containerPath))
		}
		cfg.Exec.ContainerWorkspaceDir = containerPath
	}
	if cfg.Exec.ContainerUser == "" {
		cfg.Exec.ContainerUser = "65532:65532"
	}
	if !cfg.Exec.ContainerReadOnlyRoot {
		cfg.Exec.ContainerReadOnlyRoot = true
	}
	if err := normalizeOpenCLI(workspaceRoot, &cfg.OpenCLI); err != nil {
		return nil, fmt.Errorf("parse %s: opencli: %w", p, err)
	}
	if cfg.SkillsDir == "" {
		cfg.SkillsDir = filepath.Join(workspaceRoot, "skills")
	} else if !filepath.IsAbs(cfg.SkillsDir) {
		cfg.SkillsDir = filepath.Clean(filepath.Join(workspaceRoot, cfg.SkillsDir))
	}
	if err := ensureOwnedDir(cfg.SkillsDir); err != nil {
		return nil, fmt.Errorf("parse %s: skills_dir %q: %w", p, cfg.SkillsDir, err)
	}
	if cfg.Thinker.KnowledgeMDDir == "" {
		cfg.Thinker.KnowledgeMDDir = "knowledge"
	}
	mdDir := cfg.Thinker.KnowledgeMDDir
	if !filepath.IsAbs(mdDir) {
		mdDir = filepath.Clean(filepath.Join(workspaceRoot, mdDir))
	} else {
		mdDir = filepath.Clean(mdDir)
	}
	cfg.Thinker.KnowledgeMDDir = mdDir
	if err := ensureOwnedDir(cfg.Thinker.KnowledgeMDDir); err != nil {
		return nil, fmt.Errorf("parse %s: thinker.knowledge_md_dir %q: %w", p, cfg.Thinker.KnowledgeMDDir, err)
	}
	return &cfg, nil
}

func normalizeOpenCLI(workspaceRoot string, cfg *OpenCLI) error {
	if cfg == nil {
		return nil
	}
	cfg.Command = strings.TrimSpace(cfg.Command)
	if cfg.Command == "" {
		cfg.Commands = nil
		return nil
	}
	if strings.ContainsRune(cfg.Command, os.PathSeparator) && !filepath.IsAbs(cfg.Command) {
		cfg.Command = filepath.Clean(filepath.Join(workspaceRoot, cfg.Command))
	}
	if cfg.CommandTimeoutSec <= 0 {
		cfg.CommandTimeoutSec = 90
	}
	if len(cfg.Commands) == 0 {
		return fmt.Errorf("commands allowlist is required when command is configured")
	}
	normalized := make(map[string][]string, len(cfg.Commands))
	for rawSite, rawCommands := range cfg.Commands {
		site := strings.TrimSpace(rawSite)
		if site == "" {
			return fmt.Errorf("commands contains an empty site name")
		}
		seen := make(map[string]struct{})
		for _, rawCommand := range rawCommands {
			command := strings.TrimSpace(rawCommand)
			if command == "" {
				return fmt.Errorf("commands[%q] contains an empty command", site)
			}
			if _, exists := seen[command]; exists {
				continue
			}
			seen[command] = struct{}{}
			normalized[site] = append(normalized[site], command)
		}
		if len(normalized[site]) == 0 {
			return fmt.Errorf("commands[%q] must contain at least one command", site)
		}
	}
	cfg.Commands = normalized
	return nil
}

func ensureOwnedDir(dir string) error {
	if strings.TrimSpace(dir) == "" {
		return fmt.Errorf("directory is required")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	info, err := os.Stat(dir)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("not a directory")
	}
	return nil
}
