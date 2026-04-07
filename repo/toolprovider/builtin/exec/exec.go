package execbuiltin

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/OctoSucker/octosucker/config"
	"github.com/OctoSucker/octosucker/engine/types"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ToolName is the single MCP tool for this builtin; the executable name is arguments.program.
const ToolName = "run_command"

type Runner struct {
	cfg    runnerConfig
	ensure sync.Mutex
}

type runnerConfig struct {
	backend             string
	roots               []string
	timeoutSec          int
	blacklist           []string
	runtime             string
	image               string
	name                string
	containerWorkdir    string
	readOnlyRoot        bool
	containerUser       string
	macOSSandboxProfile string
	sandboxExecPath     string
}

func NewRunner(execCfg config.Exec) (*Runner, error) {
	roots := make([]string, 0, len(execCfg.WorkspaceDirs))
	for _, r := range execCfg.WorkspaceDirs {
		abs, err := filepath.Abs(r)
		if err != nil {
			return nil, fmt.Errorf("exec builtin: invalid workspace dir %q: %w", r, err)
		}
		st, err := os.Stat(abs)
		if err != nil {
			return nil, fmt.Errorf("exec builtin: workspace dir %q: %w", abs, err)
		}
		if !st.IsDir() {
			return nil, fmt.Errorf("exec builtin: workspace dir %q is not a directory", abs)
		}
		roots = append(roots, filepath.Clean(abs))
	}
	if len(roots) == 0 {
		return nil, fmt.Errorf("exec builtin: workspace_dirs required")
	}
	if execCfg.CommandTimeoutSec <= 0 {
		return nil, fmt.Errorf("exec builtin: command_timeout_sec must be > 0")
	}
	backend := execCfg.Backend
	if backend == "" {
		backend = config.ExecBackendDocker
	}
	rc := runnerConfig{
		backend:       backend,
		roots:         roots,
		timeoutSec:    execCfg.CommandTimeoutSec,
		blacklist:     append([]string(nil), execCfg.CommandBlacklist...),
		readOnlyRoot:  execCfg.ContainerReadOnlyRoot,
		containerUser: execCfg.ContainerUser,
	}
	switch backend {
	case config.ExecBackendDocker:
		if err := configureDockerRunner(&rc, execCfg); err != nil {
			return nil, err
		}
	case config.ExecBackendMacOSSandboxExec:
		if err := configureMacOSRunner(&rc, execCfg); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("exec builtin: unknown backend %q", backend)
	}
	return &Runner{cfg: rc}, nil
}

// Name is the stable tool-provider name (Registry.providersByName key); not an MCP tool name.
func (r *Runner) Name() (string, string) {
	return "exec", "Run shell commands inside workspace (Docker or macOS sandbox)."
}

func (r *Runner) HasTool(name string) bool {
	return strings.TrimSpace(name) == ToolName
}

func (r *Runner) Tool(tool string) (*mcp.Tool, error) {
	if strings.TrimSpace(tool) != ToolName {
		return nil, fmt.Errorf("exec builtin: unknown tool %q (only %q is supported)", tool, ToolName)
	}
	desc := "Run a command: set arguments.program to argv0 (binary in the Docker sandbox image or on image PATH)"
	if r.cfg.backend == config.ExecBackendMacOSSandboxExec {
		desc = "Run a command: set arguments.program to argv0 (binary on host PATH or absolute path; runs under sandbox-exec with exec.macos_sandbox_profile)"
	}
	return &mcp.Tool{
		Name:        ToolName,
		Description: desc,
		InputSchema: ToolInputSchema(),
	}, nil
}

func (r *Runner) Invoke(ctx context.Context, localTool string, arguments map[string]any) (types.ToolResult, error) {
	if strings.TrimSpace(localTool) != ToolName {
		return types.ToolResult{Err: fmt.Errorf("exec builtin: tool must be %q", ToolName)}, fmt.Errorf("exec builtin: tool must be %q", ToolName)
	}
	return r.runCommand(ctx, arguments)
}

func (r *Runner) runCommand(ctx context.Context, args map[string]any) (types.ToolResult, error) {
	if args == nil {
		args = map[string]any{}
	}
	rawProg, ok := args["program"]
	if !ok {
		return types.ToolResult{Err: fmt.Errorf("exec builtin: argument \"program\" is required")}, fmt.Errorf("exec builtin: argument \"program\" is required")
	}
	program, ok := rawProg.(string)
	if !ok || strings.TrimSpace(program) == "" {
		return types.ToolResult{Err: fmt.Errorf("exec builtin: argument \"program\" must be non-empty string")}, fmt.Errorf("exec builtin: argument \"program\" must be non-empty string")
	}
	program = strings.TrimSpace(program)
	extraArgs, err := parseExecArgsSlice(args["args"])
	if err != nil {
		return types.ToolResult{Err: err}, err
	}

	wd := r.cfg.roots[0]
	if rawCWD, ok := args["work_dir"]; ok {
		cwd, ok := rawCWD.(string)
		if !ok || strings.TrimSpace(cwd) == "" {
			return types.ToolResult{Err: fmt.Errorf("exec builtin: argument \"work_dir\" must be non-empty string")}, fmt.Errorf("exec builtin: argument \"work_dir\" must be non-empty string")
		}
		resolved, err := resolveWorkDir(cwd, r.cfg.roots)
		if err != nil {
			return types.ToolResult{Err: err}, err
		}
		wd = resolved
	}

	extraArgs = normalizeShellArgv(program, wd, extraArgs)
	argv := append([]string{program}, extraArgs...)
	fullCmd := strings.Join(argv, " ")
	for _, blocked := range r.cfg.blacklist {
		if blocked != "" && strings.Contains(fullCmd, blocked) {
			return types.ToolResult{Err: fmt.Errorf("exec builtin: command is forbidden by blacklist: %s", fullCmd)}, fmt.Errorf("exec builtin: command is forbidden by blacklist: %s", fullCmd)
		}
	}

	timeoutSec := r.cfg.timeoutSec
	if rawTimeout, ok := args["timeout_sec"]; ok {
		timeoutSecParsed, err := parseTimeoutSec(rawTimeout)
		if err != nil {
			return types.ToolResult{Err: err}, err
		}
		timeoutSec = timeoutSecParsed
	}
	runCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
	defer cancel()

	if err := r.ensureSandbox(runCtx); err != nil {
		return types.ToolResult{Err: err}, err
	}
	switch r.cfg.backend {
	case config.ExecBackendDocker:
		return r.runDocker(runCtx, wd, program, argv, args)
	case config.ExecBackendMacOSSandboxExec:
		return r.runMacOSSandbox(runCtx, wd, program, argv, args)
	default:
		return types.ToolResult{Err: fmt.Errorf("exec builtin: unknown backend %q", r.cfg.backend)}, fmt.Errorf("exec builtin: unknown backend %q", r.cfg.backend)
	}
}

// ToolInputSchema is the JSON Schema for run_command arguments.
func ToolInputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"program": map[string]any{
				"type":        "string",
				"description": "argv0: executable name or path (e.g. opencli, npm, git, sh, bash)",
			},
			"args": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
				"description": "Argv after program. For sh/bash/zsh/dash: use [\"-c\", \"whole shell command\"] for one-liners; " +
					"a single arg is treated as a script path (sh myscript.sh). For other programs use normal argv.",
			},
			"work_dir": map[string]any{
				"type":        "string",
				"description": "Optional working directory for command execution",
			},
			"timeout_sec": map[string]any{
				"type":        "integer",
				"description": "Optional timeout in seconds",
			},
			"env": map[string]any{
				"type":                 "object",
				"description":          "Optional environment variables",
				"additionalProperties": map[string]any{"type": "string"},
			},
		},
		"additionalProperties": false,
	}
}

func (r *Runner) ToolList(ctx context.Context) ([]*mcp.Tool, error) {
	t, err := r.Tool(ToolName)
	if err != nil {
		return nil, err
	}
	return []*mcp.Tool{t}, nil
}

func parseExecArgsSlice(v any) ([]string, error) {
	if v == nil {
		return nil, nil
	}
	arr, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("exec builtin: argument \"args\" must be array of strings")
	}
	out := make([]string, len(arr))
	for i, el := range arr {
		s, ok := el.(string)
		if !ok {
			return nil, fmt.Errorf("exec builtin: argument \"args\"[%d] must be string", i)
		}
		out[i] = s
	}
	return out, nil
}

func parseTimeoutSec(v any) (int, error) {
	switch t := v.(type) {
	case float64:
		if t <= 0 {
			return 0, fmt.Errorf("exec builtin: argument \"timeout_sec\" must be > 0")
		}
		sec := int(t)
		if sec <= 0 {
			return 1, nil
		}
		return sec, nil
	case int:
		if t <= 0 {
			return 0, fmt.Errorf("exec builtin: argument \"timeout_sec\" must be > 0")
		}
		return t, nil
	default:
		return 0, fmt.Errorf("exec builtin: argument \"timeout_sec\" must be integer seconds")
	}
}

// normalizeShellArgv rewrites mistaken planner invocations such as
// sh with argv [npm, install, pkg] (sh treats npm as a script path) into
// sh -c "npm install pkg". Single-arg sh stays unchanged (sh scriptname).
func normalizeShellArgv(tool, wd string, extra []string) []string {
	tool = strings.TrimSpace(tool)
	if tool == "" || len(extra) == 0 {
		return extra
	}
	base := strings.ToLower(filepath.Base(tool))
	switch base {
	case "sh", "bash", "zsh", "dash", "ash", "ksh":
	default:
		return extra
	}
	if extra[0] == "-c" || extra[0] == "-s" {
		return extra
	}
	if len(extra) == 1 {
		return extra
	}
	first := extra[0]
	if shellFirstArgIsScriptFile(first, wd) {
		return extra
	}
	return []string{"-c", strings.Join(extra, " ")}
}

func shellFirstArgIsScriptFile(first, wd string) bool {
	if strings.Contains(first, "/") || strings.HasPrefix(first, "./") {
		return true
	}
	p := filepath.Join(wd, first)
	if st, err := os.Stat(p); err == nil && !st.IsDir() {
		return true
	}
	if filepath.IsAbs(first) {
		if st, err := os.Stat(first); err == nil && !st.IsDir() {
			return true
		}
	}
	return false
}

func resolveWorkDir(workDir string, roots []string) (string, error) {
	path := filepath.Clean(workDir)
	var abs string
	if filepath.IsAbs(path) {
		var err error
		abs, err = filepath.Abs(path)
		if err != nil {
			return "", fmt.Errorf("exec builtin: invalid work_dir %q: %w", workDir, err)
		}
	} else {
		abs = filepath.Join(roots[0], path)
	}
	abs = filepath.Clean(abs)
	for _, root := range roots {
		rel, err := filepath.Rel(root, abs)
		if err != nil {
			continue
		}
		if rel == "." || (!strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel)) {
			return abs, nil
		}
	}
	return "", fmt.Errorf("exec builtin: work_dir %q is outside allowed workspace_dirs", workDir)
}

func exitCodeFromError(err error) int {
	if ee, ok := err.(*exec.ExitError); ok {
		return ee.ExitCode()
	}
	return -1
}

func formatExecRunError(tool string, err error, stderr string) error {
	stderr = strings.TrimSpace(stderr)
	raw := strings.TrimSpace(err.Error())
	switch {
	case stderr == "":
		return fmt.Errorf("exec builtin: %s failed (exit_code=%d): %s", tool, exitCodeFromError(err), raw)
	case raw == "":
		return fmt.Errorf("exec builtin: %s failed (exit_code=%d): %s", tool, exitCodeFromError(err), stderr)
	case strings.Contains(stderr, raw):
		return fmt.Errorf("exec builtin: %s failed (exit_code=%d): %s", tool, exitCodeFromError(err), stderr)
	default:
		return fmt.Errorf("exec builtin: %s failed (exit_code=%d): %s | run error: %s", tool, exitCodeFromError(err), stderr, raw)
	}
}
