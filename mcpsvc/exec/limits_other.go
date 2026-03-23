//go:build !linux && !darwin

package execmcp

func applySandboxLimits(limits SandboxLimits) (restore func(), err error) {
	return func() {}, nil
}
