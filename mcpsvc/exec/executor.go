package execmcp

import "context"

type Executor interface {
	Run(ctx context.Context, argv []string, workDir string, env []string, timeoutSec int, limits SandboxLimits) (*RunResult, error)
}

type RunResult struct {
	Stdout           []byte
	Stderr           []byte
	ExitCode         int
	Timeout          bool
	SandboxViolation bool
	Message          string
}
