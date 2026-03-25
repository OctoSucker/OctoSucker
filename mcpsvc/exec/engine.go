package execmcp

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
)

const maxCapture = 512 * 1024

type Engine struct {
	cfg      Config
	executor Executor
}

func NewEngine(cfg Config) *Engine {
	bind := cfg.HostBindRoots
	return &Engine{
		cfg: cfg,
		executor: &DockerExecutor{
			mgr: sandboxContainerManager{
				runtime:      cfg.Container.Runtime,
				image:        cfg.Container.Image,
				name:         cfg.Container.Name,
				workspaceDir: cfg.Container.WorkspaceDir,
				readOnlyRoot: cfg.Container.ReadOnlyRoot,
				roots:        cfg.Roots,
				bindRoots:    bind,
			},
			containerUser: cfg.Container.User,
		},
	}
}

func (e *Engine) isBlacklisted(cmdLine string) bool {
	for _, pattern := range e.cfg.Blacklist {
		if strings.Contains(cmdLine, pattern) {
			return true
		}
	}
	return false
}

func splitCommand(s string) []string {
	return strings.Fields(s)
}

func trimOut(b []byte) string {
	if len(b) <= maxCapture {
		return string(b)
	}
	return string(b[:maxCapture]) + "\n... [truncated]"
}

func (e *Engine) RunCommand(ctx context.Context, command, workDir string, timeoutSec int, env map[string]string) (map[string]any, error) {
	if command == "" {
		return nil, fmt.Errorf("command is required and must be non-empty")
	}
	roots := e.cfg.Roots
	if len(roots) == 0 {
		return nil, fmt.Errorf("execmcp: workspace_dirs not configured")
	}
	if e.isBlacklisted(command) {
		return nil, fmt.Errorf("execmcp: command is forbidden by blacklist: %s", command)
	}
	wd := workDir
	if wd == "" {
		wd = roots[0]
	} else {
		resolved, err := resolveWorkDir(wd, roots)
		if err != nil {
			return nil, err
		}
		wd = resolved
	}
	to := e.cfg.TimeoutSec
	if timeoutSec > 0 {
		to = timeoutSec
	}
	argv := splitCommand(command)
	if len(argv) == 0 {
		return nil, fmt.Errorf("command produced no executable")
	}
	if isRmCommand(argv) {
		paths := parseRmPaths(argv)
		if len(paths) == 0 {
			return nil, fmt.Errorf("rm requires at least one path argument")
		}
		var absPaths []string
		for _, p := range paths {
			abs, err := resolvePathInWorkspace(p, wd, roots)
			if err != nil {
				return nil, err
			}
			absPaths = append(absPaths, abs)
		}
		moved, err := moveToTrash(wd, absPaths)
		if err != nil {
			return nil, fmt.Errorf("move to trash: %w", err)
		}
		trashDir := filepath.Join(wd, trashDirName)
		msg := fmt.Sprintf("rm is redirected to trash: %d item(s) moved to %s (not deleted)", len(moved), trashDir)
		return map[string]any{
			"success":       true,
			"stdout":        msg,
			"stderr":        "",
			"exit_code":     0,
			"work_dir":      wd,
			"trash_dir":     trashDir,
			"moved":         moved,
			"rm_redirected": true,
		}, nil
	}
	envList := make([]string, 0, len(env))
	for k, v := range env {
		envList = append(envList, k+"="+v)
	}
	result, err := e.executor.Run(ctx, argv, wd, envList, to, SandboxLimits{})
	if err != nil {
		return nil, fmt.Errorf("run_command: %w", err)
	}
	out := map[string]any{
		"success":   result.ExitCode == 0 && !result.Timeout,
		"stdout":    trimOut(result.Stdout),
		"stderr":    trimOut(result.Stderr),
		"exit_code": result.ExitCode,
		"work_dir":  wd,
	}
	if result.Timeout {
		out["timeout"] = true
		out["message"] = result.Message
	}
	if result.SandboxViolation {
		out["sandbox_violation"] = true
		out["message"] = result.Message
	}
	if result.Message != "" && out["message"] == nil {
		out["message"] = result.Message
	}
	return out, nil
}
