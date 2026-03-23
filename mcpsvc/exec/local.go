package execmcp

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"
)

type LocalExecutor struct{}

func (e *LocalExecutor) Run(ctx context.Context, argv []string, workDir string, env []string, timeoutSec int, limits SandboxLimits) (*RunResult, error) {
	if timeoutSec <= 0 {
		timeoutSec = 30
	}
	runCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(runCtx, argv[0], argv[1:]...)
	cmd.Dir = workDir
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	needLimits := limits.CPUsec > 0 || limits.MemoryMB > 0 || limits.MaxProcs > 0 || limits.MaxOpenFiles > 0
	if needLimits {
		restore, err := applySandboxLimits(limits)
		if err != nil {
			return nil, fmt.Errorf("sandbox limits: %w", err)
		}
		defer restore()
	}

	err := cmd.Run()
	out := stdout.Bytes()
	errOut := stderr.Bytes()
	exitCode := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			exitCode = ee.ExitCode()
		}
	}

	result := &RunResult{
		Stdout:   out,
		Stderr:   errOut,
		ExitCode: exitCode,
	}
	if runCtx.Err() == context.DeadlineExceeded {
		result.Timeout = true
		result.Message = "command timed out and was killed"
	}
	return result, nil
}
