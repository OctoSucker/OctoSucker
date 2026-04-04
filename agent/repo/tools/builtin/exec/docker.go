package execbuiltin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/OctoSucker/agent/internal/config"
	"github.com/OctoSucker/agent/pkg/ports"
)

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

func configureDockerRunner(rc *runnerConfig, execCfg config.Exec) error {
	if execCfg.ContainerRuntime == "" || execCfg.ContainerImage == "" || execCfg.ContainerName == "" || execCfg.ContainerWorkspaceDir == "" {
		return fmt.Errorf("exec builtin: container runtime/image/name/workspace_dir are required for backend %q", config.ExecBackendDocker)
	}
	rc.runtime = execCfg.ContainerRuntime
	rc.image = execCfg.ContainerImage
	rc.name = execCfg.ContainerName
	rc.containerWorkdir = execCfg.ContainerWorkspaceDir
	return nil
}

func (r *Runner) runDocker(ctx context.Context, wd, tool string, argv []string, args map[string]any) (ports.ToolResult, error) {
	insideWD, err := mapWorkDirToContainer(wd, r.cfg.roots, r.cfg.containerWorkdir)
	if err != nil {
		return ports.ToolResult{Err: err}, err
	}

	dockerArgs := []string{"exec", "--workdir", insideWD}
	if r.cfg.containerUser != "" {
		dockerArgs = append(dockerArgs, "--user", r.cfg.containerUser)
	}
	if rawEnv, ok := args["env"]; ok && rawEnv != nil {
		envMap, ok := rawEnv.(map[string]any)
		if !ok {
			return ports.ToolResult{Err: fmt.Errorf("exec builtin: argument \"env\" must be object")}, fmt.Errorf("exec builtin: argument \"env\" must be object")
		}
		for k, v := range envMap {
			vs, ok := v.(string)
			if !ok {
				return ports.ToolResult{Err: fmt.Errorf("exec builtin: env value for %q must be string", k)}, fmt.Errorf("exec builtin: env value for %q must be string", k)
			}
			dockerArgs = append(dockerArgs, "-e", k+"="+vs)
		}
	}
	dockerArgs = append(dockerArgs, r.cfg.name)
	dockerArgs = append(dockerArgs, argv...)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, r.cfg.runtime, dockerArgs...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	if err != nil {
		runErr := formatExecRunError(tool, err, stderr.String())
		return ports.ToolResult{Err: runErr}, runErr
	}
	return ports.ToolResult{
		Output: map[string]any{
			"stdout":    stdout.String(),
			"stderr":    stderr.String(),
			"exit_code": 0,
			"work_dir":  wd,
		},
	}, nil
}

func (r *Runner) ensureSandbox(ctx context.Context) error {
	if r.cfg.backend != config.ExecBackendDocker {
		return nil
	}
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
