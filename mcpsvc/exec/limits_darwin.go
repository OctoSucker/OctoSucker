//go:build darwin

package execmcp

import (
	"fmt"
	"syscall"
)

func applySandboxLimits(limits SandboxLimits) (restore func(), err error) {
	var restores []func()
	doRestore := func() {
		for i := len(restores) - 1; i >= 0; i-- {
			restores[i]()
		}
	}
	getAndSet := func(resource int, soft, hard uint64) error {
		var rlim syscall.Rlimit
		if err := syscall.Getrlimit(resource, &rlim); err != nil {
			return err
		}
		prev := rlim
		restores = append(restores, func() { _ = syscall.Setrlimit(resource, &prev) })
		rlim.Cur = soft
		rlim.Max = hard
		return syscall.Setrlimit(resource, &rlim)
	}
	if limits.CPUsec > 0 {
		if err := getAndSet(syscall.RLIMIT_CPU, uint64(limits.CPUsec), uint64(limits.CPUsec)); err != nil {
			doRestore()
			return nil, fmt.Errorf("RLIMIT_CPU: %w", err)
		}
	}
	if limits.MemoryMB > 0 {
		bytes := uint64(limits.MemoryMB) * 1024 * 1024
		if err := getAndSet(syscall.RLIMIT_AS, bytes, bytes); err != nil {
			doRestore()
			return nil, fmt.Errorf("RLIMIT_AS: %w", err)
		}
	}
	if limits.MaxOpenFiles > 0 {
		if err := getAndSet(syscall.RLIMIT_NOFILE, uint64(limits.MaxOpenFiles), uint64(limits.MaxOpenFiles)); err != nil {
			doRestore()
			return nil, fmt.Errorf("RLIMIT_NOFILE: %w", err)
		}
	}
	return doRestore, nil
}
