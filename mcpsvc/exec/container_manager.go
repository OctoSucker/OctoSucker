package execmcp

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// sandboxContainerManager keeps a long-lived sandbox container: ensure it exists,
// start it if stopped, or create it with workspace volume mounts.
type sandboxContainerManager struct {
	mu           sync.Mutex
	runtime      string
	image        string
	name         string
	workspaceDir string
	readOnlyRoot bool
	roots        []string
	// bindRoots: host-side paths for docker -v; same length as roots.
	bindRoots []string
}

func (m *sandboxContainerManager) ensure(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.runtime == "" {
		return fmt.Errorf("container runtime is empty")
	}
	if m.image == "" {
		return fmt.Errorf("container image is empty")
	}
	if m.name == "" {
		return fmt.Errorf("container name is empty")
	}
	if m.workspaceDir == "" {
		return fmt.Errorf("container workspace dir is empty")
	}

	exists, running, err := m.inspect(ctx)
	if err != nil {
		return err
	}
	if exists && running {
		return nil
	}
	if exists && !running {
		return m.start(ctx)
	}
	return m.create(ctx)
}

func (m *sandboxContainerManager) inspect(ctx context.Context) (exists, running bool, err error) {
	cmd := exec.CommandContext(ctx, m.runtime, "container", "inspect", "-f", "{{.State.Running}}", m.name)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() != 0 {
			errText := strings.TrimSpace(stderr.String())
			if strings.Contains(errText, "No such container") {
				return false, false, nil
			}
		}
		return false, false, fmt.Errorf("inspect sandbox container %q failed: %w", m.name, err)
	}
	status := strings.TrimSpace(stdout.String())
	return true, status == "true", nil
}

func (m *sandboxContainerManager) start(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, m.runtime, "start", m.name)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("start sandbox container %q failed: %s: %w", m.name, strings.TrimSpace(stderr.String()), err)
	}
	return nil
}

func (m *sandboxContainerManager) create(ctx context.Context) error {
	args := []string{
		"run", "-d",
		"--name", m.name,
		"--network", "bridge",
		"--cap-drop", "ALL",
		"--security-opt", "no-new-privileges:true",
		"--pids-limit", "256",
	}
	if m.readOnlyRoot {
		args = append(args, "--read-only", "--tmpfs", "/tmp:rw,noexec,nosuid,size=64m", "--tmpfs", "/run:rw,nosuid,size=16m")
	}
	for i := range m.roots {
		dst := filepath.ToSlash(filepath.Join(m.workspaceDir, strconv.Itoa(i)))
		src := bindSourceForNestedDocker(m.bindRoots[i])
		args = append(args, "-v", src+":"+dst+":rw")
	}
	args = append(args, m.image)

	cmd := exec.CommandContext(ctx, m.runtime, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("create sandbox container %q failed: %s: %w", m.name, strings.TrimSpace(stderr.String()), err)
	}
	return nil
}
