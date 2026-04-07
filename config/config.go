// Package config loads workspace config.json (OpenAI, MCP, exec sandbox, Telegram, skills_dir).
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
	MCPEndpoint []string `json:"mcp_endpoint"`
	Exec        Exec     `json:"exec"`
	HTTP        HTTP     `json:"http"`
	Telegram    Telegram `json:"telegram"`
	SkillsDir   string   `json:"skills_dir"` // based on workspace root
	Thinker     Thinker  `json:"thinker"`
}

// Thinker configures the knowledge-only agent (Markdown corpus directory).
type Thinker struct {
	KnowledgeMDDir string `json:"knowledge_md_dir"`
}

type OpenAI struct {
	APIKey         string `json:"api_key"`
	BaseURL        string `json:"base_url"`
	Model          string `json:"model"`
	EmbeddingModel string `json:"embedding_model"`
}

type HTTP struct {
	Listen string `json:"listen"`
}

type Telegram struct {
	BotToken       string  `json:"bot_token"`
	DefaultChatID  int64   `json:"default_chat_id"`
	AllowedChatIDs []int64 `json:"allowed_chat_ids,omitempty"`
}

// Exec sandbox backends (exec.backend JSON field).
const (
	ExecBackendDocker           = "docker"
	ExecBackendMacOSSandboxExec = "macos_sandbox_exec"
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
	case ExecBackendDocker, ExecBackendMacOSSandboxExec:
	default:
		return nil, fmt.Errorf("parse %s: exec.backend must be %q or %q", p, ExecBackendDocker, ExecBackendMacOSSandboxExec)
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
	if cfg.SkillsDir == "" {
		cfg.SkillsDir = filepath.Join(workspaceRoot, "agent", "workspace", "skills")
	} else if !filepath.IsAbs(cfg.SkillsDir) {
		cfg.SkillsDir = filepath.Clean(filepath.Join(workspaceRoot, cfg.SkillsDir))
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
	return &cfg, nil
}
