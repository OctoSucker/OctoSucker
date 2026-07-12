package tools

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func preflightTool(tool string, arguments map[string]any) error {
	switch strings.TrimSpace(tool) {
	case "run_command":
		return preflightRunCommand(arguments)
	default:
		return nil
	}
}

func preflightRunCommand(arguments map[string]any) error {
	if arguments == nil {
		return nil
	}
	program, _ := arguments["program"].(string)
	program = strings.TrimSpace(program)
	if program == "" {
		return nil
	}
	if isShell(program) {
		return preflightExecutable(program)
	}
	return preflightExecutable(program)
}

func preflightExecutable(program string) error {
	if strings.ContainsRune(program, os.PathSeparator) || filepath.IsAbs(program) {
		if _, err := os.Stat(program); err != nil {
			return fmt.Errorf("preflight: executable %q is not available: %w", program, err)
		}
		return nil
	}
	if _, err := exec.LookPath(program); err != nil {
		return fmt.Errorf("preflight: executable %q is not on PATH", program)
	}
	return nil
}

func isShell(program string) bool {
	switch filepath.Base(strings.TrimSpace(program)) {
	case "sh", "bash", "zsh", "dash":
		return true
	default:
		return false
	}
}
