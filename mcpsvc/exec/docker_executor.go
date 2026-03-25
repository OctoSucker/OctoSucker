package execmcp

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type DockerExecutor struct {
	mgr           sandboxContainerManager
	containerUser string
}

func (e *DockerExecutor) Run(ctx context.Context, argv []string, workDir string, env []string, timeoutSec int, _ SandboxLimits) (*RunResult, error) {
	if timeoutSec <= 0 {
		timeoutSec = 30
	}
	runCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
	defer cancel()

	if err := e.mgr.ensure(runCtx); err != nil {
		return nil, err
	}

	args, err := e.buildDockerExecArgs(argv, workDir, env)
	if err != nil {
		return nil, err
	}

	cmd := exec.CommandContext(runCtx, e.mgr.runtime, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	result := &RunResult{
		Stdout:   stdout.Bytes(),
		Stderr:   stderr.Bytes(),
		ExitCode: 0,
	}
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			result.ExitCode = ee.ExitCode()
		} else {
			return nil, fmt.Errorf("container runtime %q failed: %w", e.mgr.runtime, err)
		}
	}
	if runCtx.Err() == context.DeadlineExceeded {
		result.Timeout = true
		result.Message = "command timed out and was killed"
	}
	return result, nil
}

func (e *DockerExecutor) buildDockerExecArgs(argv []string, workDir string, env []string) ([]string, error) {
	if e.mgr.runtime == "" {
		return nil, fmt.Errorf("container runtime is empty")
	}
	if e.mgr.name == "" {
		return nil, fmt.Errorf("container name is empty")
	}
	if e.mgr.workspaceDir == "" {
		return nil, fmt.Errorf("container workspace dir is empty")
	}

	insideWD, err := mapWorkDirToContainer(workDir, e.mgr.roots, e.mgr.workspaceDir)
	if err != nil {
		return nil, err
	}

	args := []string{
		"exec",
		"--workdir", insideWD,
	}
	if e.containerUser != "" {
		args = append(args, "--user", e.containerUser)
	}
	for _, kv := range env {
		args = append(args, "-e", kv)
	}
	args = append(args, e.mgr.name)
	args = append(args, argv...)
	return args, nil
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
	return "", fmt.Errorf("work_dir %q is outside configured roots", workDir)
}
