package execmcp

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Roots          []string
	TimeoutSec     int
	Blacklist      []string
	SandboxEnabled bool
	SandboxLimits  SandboxLimits
}

func LoadFromEnv() (Config, error) {
	raw := os.Getenv("EXEC_WORKSPACE_DIRS")
	if raw == "" {
		return Config{}, fmt.Errorf("EXEC_WORKSPACE_DIRS is required (comma-separated absolute or ~ paths)")
	}
	parts := strings.Split(raw, ",")
	var paths []string
	for _, p := range parts {
		if p != "" {
			paths = append(paths, p)
		}
	}
	roots, err := normalizeRoots(paths)
	if err != nil {
		return Config{}, err
	}
	if len(roots) == 0 {
		return Config{}, fmt.Errorf("EXEC_WORKSPACE_DIRS: no valid paths after normalization")
	}
	timeout := 30
	if s := os.Getenv("EXEC_COMMAND_TIMEOUT_SEC"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n <= 0 {
			return Config{}, fmt.Errorf("EXEC_COMMAND_TIMEOUT_SEC: invalid integer")
		}
		timeout = n
	}
	var blacklist []string
	if s := os.Getenv("EXEC_COMMAND_BLACKLIST"); s != "" {
		for _, frag := range strings.Split(s, ",") {
			if frag != "" {
				blacklist = append(blacklist, frag)
			}
		}
	}
	lim := SandboxLimits{}
	sbx := os.Getenv("EXEC_SANDBOX_ENABLED")
	enabled := sbx == "1" || strings.EqualFold(sbx, "true")
	if enabled {
		lim = parseSandboxLimitsFromEnv()
	}
	return Config{
		Roots:          roots,
		TimeoutSec:     timeout,
		Blacklist:      blacklist,
		SandboxEnabled: enabled,
		SandboxLimits:  lim,
	}, nil
}

func parseSandboxLimitsFromEnv() SandboxLimits {
	var l SandboxLimits
	if s := os.Getenv("EXEC_SANDBOX_CPU_SEC"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			l.CPUsec = n
		}
	}
	if s := os.Getenv("EXEC_SANDBOX_MEMORY_MB"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			l.MemoryMB = n
		}
	}
	if s := os.Getenv("EXEC_SANDBOX_MAX_PROCS"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			l.MaxProcs = n
		}
	}
	if s := os.Getenv("EXEC_SANDBOX_MAX_OPEN_FILES"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			l.MaxOpenFiles = n
		}
	}
	if s := os.Getenv("EXEC_SANDBOX_NETWORK"); s == "none" || s == "full" {
		l.Network = s
	}
	return l
}
