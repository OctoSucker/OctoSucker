package execmcp

type RunResult struct {
	Stdout           []byte
	Stderr           []byte
	ExitCode         int
	Timeout          bool
	SandboxViolation bool
	Message          string
}
