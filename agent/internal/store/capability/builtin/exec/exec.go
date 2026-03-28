package execbuiltin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/OctoSucker/agent/internal/config"
	"github.com/OctoSucker/agent/pkg/ports"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const CapabilityName = "exec"

type Runner struct {
	cfg    dockerConfig
	ensure sync.Mutex
}

type dockerConfig struct {
	roots            []string
	timeoutSec       int
	blacklist        []string
	runtime          string
	image            string
	name             string
	containerWorkdir string
	readOnlyRoot     bool
	containerUser    string
}

type containerInspect struct {
	Config struct {
		Image string `json:"Image"`
		User  string `json:"User"`
	} `json:"Config"`
	State struct {
		Running bool `json:"Running"`
	} `json:"State"`
	HostConfig struct {
		ReadonlyRootfs bool     `json:"ReadonlyRootfs"`
		CapDrop        []string `json:"CapDrop"`
		SecurityOpt    []string `json:"SecurityOpt"`
		PidsLimit      int64    `json:"PidsLimit"`
		NetworkMode    string   `json:"NetworkMode"`
	} `json:"HostConfig"`
	Mounts []struct {
		Type        string `json:"Type"`
		Source      string `json:"Source"`
		Destination string `json:"Destination"`
		RW          bool   `json:"RW"`
	} `json:"Mounts"`
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
	if execCfg.ContainerRuntime == "" || execCfg.ContainerImage == "" || execCfg.ContainerName == "" || execCfg.ContainerWorkspaceDir == "" {
		return nil, fmt.Errorf("exec builtin: container runtime/image/name/workspace_dir are required")
	}
	return &Runner{
		cfg: dockerConfig{
			roots:            roots,
			timeoutSec:       execCfg.CommandTimeoutSec,
			blacklist:        append([]string(nil), execCfg.CommandBlacklist...),
			runtime:          execCfg.ContainerRuntime,
			image:            execCfg.ContainerImage,
			name:             execCfg.ContainerName,
			containerWorkdir: execCfg.ContainerWorkspaceDir,
			readOnlyRoot:     execCfg.ContainerReadOnlyRoot,
			containerUser:    execCfg.ContainerUser,
		},
	}, nil
}

func (r *Runner) Name() string {
	return CapabilityName
}

func (r *Runner) HasTool(name string) bool {
	return strings.TrimSpace(name) != ""
}

func (r *Runner) Tool(tool string) (*mcp.Tool, error) {
	name := strings.TrimSpace(tool)
	if name == "" {
		return nil, fmt.Errorf("exec builtin: tool (command name) is required")
	}
	return &mcp.Tool{
		Name:        name,
		Description: "Run this argv0 inside the sandbox container (binary must exist in the image)",
		InputSchema: ToolInputSchema(),
	}, nil
}

func (r *Runner) Invoke(ctx context.Context, inv ports.CapabilityInvocation) (ports.ToolResult, error) {
	tool := strings.TrimSpace(inv.Tool)
	if tool == "" {
		return ports.ToolResult{}, fmt.Errorf("exec builtin: tool (command name) is required")
	}
	return r.runCommand(ctx, tool, inv.Arguments)
}

func (r *Runner) runCommand(ctx context.Context, tool string, args map[string]any) (ports.ToolResult, error) {
	if args == nil {
		args = map[string]any{}
	}
	extraArgs, err := parseExecArgsSlice(args["args"])
	if err != nil {
		return ports.ToolResult{}, err
	}
	argv := append([]string{tool}, extraArgs...)
	fullCmd := strings.Join(argv, " ")
	for _, blocked := range r.cfg.blacklist {
		if blocked != "" && strings.Contains(fullCmd, blocked) {
			return ports.ToolResult{}, fmt.Errorf("exec builtin: command is forbidden by blacklist: %s", fullCmd)
		}
	}

	wd := r.cfg.roots[0]
	if rawCWD, ok := args["work_dir"]; ok {
		cwd, ok := rawCWD.(string)
		if !ok || strings.TrimSpace(cwd) == "" {
			return ports.ToolResult{}, fmt.Errorf("exec builtin: argument \"work_dir\" must be non-empty string")
		}
		resolved, err := resolveWorkDir(cwd, r.cfg.roots)
		if err != nil {
			return ports.ToolResult{}, err
		}
		wd = resolved
	}

	timeoutSec := r.cfg.timeoutSec
	if rawTimeout, ok := args["timeout_sec"]; ok {
		timeoutSecParsed, err := parseTimeoutSec(rawTimeout)
		if err != nil {
			return ports.ToolResult{}, err
		}
		timeoutSec = timeoutSecParsed
	}
	runCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
	defer cancel()

	if err := r.ensureSandbox(runCtx); err != nil {
		return ports.ToolResult{}, err
	}
	insideWD, err := mapWorkDirToContainer(wd, r.cfg.roots, r.cfg.containerWorkdir)
	if err != nil {
		return ports.ToolResult{}, err
	}

	dockerArgs := []string{"exec", "--workdir", insideWD}
	if r.cfg.containerUser != "" {
		dockerArgs = append(dockerArgs, "--user", r.cfg.containerUser)
	}
	if rawEnv, ok := args["env"]; ok && rawEnv != nil {
		envMap, ok := rawEnv.(map[string]any)
		if !ok {
			return ports.ToolResult{}, fmt.Errorf("exec builtin: argument \"env\" must be object")
		}
		for k, v := range envMap {
			vs, ok := v.(string)
			if !ok {
				return ports.ToolResult{}, fmt.Errorf("exec builtin: env value for %q must be string", k)
			}
			dockerArgs = append(dockerArgs, "-e", k+"="+vs)
		}
	}
	dockerArgs = append(dockerArgs, r.cfg.name)
	dockerArgs = append(dockerArgs, argv...)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := exec.CommandContext(runCtx, r.cfg.runtime, dockerArgs...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	if err != nil {
		return ports.ToolResult{}, fmt.Errorf("exec builtin: %s failed (exit_code=%d): %s", tool, exitCodeFromError(err), strings.TrimSpace(stderr.String()))
	}
	return ports.ToolResult{
		OK: true,
		Output: map[string]any{
			"stdout":    stdout.String(),
			"stderr":    stderr.String(),
			"exit_code": 0,
			"work_dir":  wd,
		},
	}, nil
}

func (r *Runner) ensureSandbox(ctx context.Context) error {
	r.ensure.Lock()
	defer r.ensure.Unlock()

	inspected, err := r.inspectSandbox(ctx)
	if err != nil {
		return err
	}
	if inspected != nil {
		if err := r.validateSandbox(inspected); err != nil {
			if err := r.removeSandbox(ctx); err != nil {
				return err
			}
			inspected = nil
		}
	}
	if inspected != nil {
		if inspected.State.Running {
			return nil
		}
		start := exec.CommandContext(ctx, r.cfg.runtime, "start", r.cfg.name)
		var startErr bytes.Buffer
		start.Stderr = &startErr
		if err := start.Run(); err != nil {
			return fmt.Errorf("exec builtin: start sandbox container %q failed: %s: %w", r.cfg.name, strings.TrimSpace(startErr.String()), err)
		}
		return nil
	}

	createArgs := []string{
		"run", "-d", "--name", r.cfg.name,
		"--network", "bridge",
		"--cap-drop", "ALL",
		"--security-opt", "no-new-privileges:true",
		"--pids-limit", "256",
	}
	if r.cfg.readOnlyRoot {
		createArgs = append(createArgs, "--read-only", "--tmpfs", "/tmp:rw,noexec,nosuid,size=64m", "--tmpfs", "/run:rw,nosuid,size=16m")
	}
	for idx, root := range r.cfg.roots {
		dst := filepath.ToSlash(filepath.Join(r.cfg.containerWorkdir, strconv.Itoa(idx)))
		createArgs = append(createArgs, "-v", root+":"+dst+":rw")
	}
	createArgs = append(createArgs, r.cfg.image)
	create := exec.CommandContext(ctx, r.cfg.runtime, createArgs...)
	var createErr bytes.Buffer
	create.Stderr = &createErr
	if err := create.Run(); err != nil {
		return fmt.Errorf("exec builtin: create sandbox container %q failed: %s: %w", r.cfg.name, strings.TrimSpace(createErr.String()), err)
	}
	return nil
}

func (r *Runner) inspectSandbox(ctx context.Context) (*containerInspect, error) {
	inspect := exec.CommandContext(ctx, r.cfg.runtime, "container", "inspect", r.cfg.name)
	var out, errOut bytes.Buffer
	inspect.Stdout = &out
	inspect.Stderr = &errOut
	if err := inspect.Run(); err != nil {
		msg := strings.ToLower(strings.TrimSpace(errOut.String()))
		if strings.Contains(msg, "no such container") {
			return nil, nil
		}
		return nil, fmt.Errorf("exec builtin: inspect sandbox container %q failed: %s: %w", r.cfg.name, strings.TrimSpace(errOut.String()), err)
	}
	var inspected []containerInspect
	if err := json.Unmarshal(out.Bytes(), &inspected); err != nil {
		return nil, fmt.Errorf("exec builtin: parse inspect output for container %q: %w", r.cfg.name, err)
	}
	if len(inspected) != 1 {
		return nil, fmt.Errorf("exec builtin: inspect output for container %q has %d entries", r.cfg.name, len(inspected))
	}
	return &inspected[0], nil
}

func (r *Runner) removeSandbox(ctx context.Context) error {
	remove := exec.CommandContext(ctx, r.cfg.runtime, "rm", "-f", r.cfg.name)
	var errOut bytes.Buffer
	remove.Stderr = &errOut
	if err := remove.Run(); err != nil {
		return fmt.Errorf("exec builtin: remove mismatched sandbox container %q failed: %s: %w", r.cfg.name, strings.TrimSpace(errOut.String()), err)
	}
	return nil
}

func (r *Runner) validateSandbox(inspected *containerInspect) error {
	if inspected.Config.Image != r.cfg.image {
		return fmt.Errorf("exec builtin: sandbox container %q image mismatch: have %q want %q", r.cfg.name, inspected.Config.Image, r.cfg.image)
	}
	if inspected.HostConfig.NetworkMode != "bridge" {
		return fmt.Errorf("exec builtin: sandbox container %q network mode mismatch: have %q want %q", r.cfg.name, inspected.HostConfig.NetworkMode, "bridge")
	}
	if inspected.HostConfig.ReadonlyRootfs != r.cfg.readOnlyRoot {
		return fmt.Errorf("exec builtin: sandbox container %q readonly root mismatch: have %v want %v", r.cfg.name, inspected.HostConfig.ReadonlyRootfs, r.cfg.readOnlyRoot)
	}
	if inspected.HostConfig.PidsLimit != 256 {
		return fmt.Errorf("exec builtin: sandbox container %q pids limit mismatch: have %d want %d", r.cfg.name, inspected.HostConfig.PidsLimit, 256)
	}
	if !sameSetStrings(inspected.HostConfig.CapDrop, []string{"ALL"}) {
		return fmt.Errorf("exec builtin: sandbox container %q cap_drop mismatch: have %v want %v", r.cfg.name, inspected.HostConfig.CapDrop, []string{"ALL"})
	}
	if !sameSetStrings(inspected.HostConfig.SecurityOpt, []string{"no-new-privileges:true"}) {
		return fmt.Errorf("exec builtin: sandbox container %q security_opt mismatch: have %v want %v", r.cfg.name, inspected.HostConfig.SecurityOpt, []string{"no-new-privileges:true"})
	}
	if r.cfg.containerUser != "" && inspected.Config.User != r.cfg.containerUser {
		return fmt.Errorf("exec builtin: sandbox container %q user mismatch: have %q want %q", r.cfg.name, inspected.Config.User, r.cfg.containerUser)
	}
	if err := validateSandboxMounts(inspected.Mounts, r.cfg.roots, r.cfg.containerWorkdir); err != nil {
		return fmt.Errorf("exec builtin: sandbox container %q mount mismatch: %w", r.cfg.name, err)
	}
	return nil
}

func validateSandboxMounts(mounts []struct {
	Type        string `json:"Type"`
	Source      string `json:"Source"`
	Destination string `json:"Destination"`
	RW          bool   `json:"RW"`
}, roots []string, containerWorkdir string) error {
	if len(mounts) != len(roots) {
		return fmt.Errorf("mount count mismatch: have %d want %d", len(mounts), len(roots))
	}
	expectedByDst := make(map[string]string, len(roots))
	for idx, root := range roots {
		dst := filepath.ToSlash(filepath.Join(containerWorkdir, strconv.Itoa(idx)))
		expectedByDst[dst] = filepath.Clean(root)
	}
	seen := make(map[string]struct{}, len(mounts))
	for _, m := range mounts {
		wantSource, ok := expectedByDst[m.Destination]
		if !ok {
			return fmt.Errorf("unexpected destination %q", m.Destination)
		}
		if m.Type != "bind" {
			return fmt.Errorf("destination %q type mismatch: have %q want %q", m.Destination, m.Type, "bind")
		}
		if !m.RW {
			return fmt.Errorf("destination %q mount mode mismatch: have read-only want read-write", m.Destination)
		}
		if !sameHostPath(m.Source, wantSource) {
			return fmt.Errorf("destination %q source mismatch: have %q want %q", m.Destination, m.Source, wantSource)
		}
		seen[m.Destination] = struct{}{}
	}
	if len(seen) != len(expectedByDst) {
		return fmt.Errorf("mounted destinations mismatch")
	}
	return nil
}

func sameHostPath(actual, expected string) bool {
	act := filepath.Clean(actual)
	exp := filepath.Clean(expected)
	if act == exp {
		return true
	}
	// Docker Desktop on macOS may expose host paths as /host_mnt/<absolute path>.
	if strings.HasPrefix(act, "/host_mnt") {
		trimmed := strings.TrimPrefix(act, "/host_mnt")
		if trimmed == exp {
			return true
		}
	}
	return false
}

func sameSetStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	aa := append([]string(nil), a...)
	bb := append([]string(nil), b...)
	sort.Strings(aa)
	sort.Strings(bb)
	for i := range aa {
		if aa[i] != bb[i] {
			return false
		}
	}
	return true
}

// ToolInputSchema is the JSON Schema for exec step arguments (any argv0).
func ToolInputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"args": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "Additional argv after the program name (flags, paths, etc.)",
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
	return nil, nil
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

func mapWorkDirToContainer(workDir string, roots []string, containerRoot string) (string, error) {
	for i, root := range roots {
		rel, err := filepath.Rel(root, workDir)
		if err != nil {
			continue
		}
		if rel == "." {
			return filepath.ToSlash(filepath.Join(containerRoot, strconv.Itoa(i))), nil
		}
		if rel != ".." && !filepath.IsAbs(rel) && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return filepath.ToSlash(filepath.Join(containerRoot, strconv.Itoa(i), rel)), nil
		}
	}
	return "", fmt.Errorf("exec builtin: work_dir %q is outside configured roots", workDir)
}

func exitCodeFromError(err error) int {
	if ee, ok := err.(*exec.ExitError); ok {
		return ee.ExitCode()
	}
	return -1
}
