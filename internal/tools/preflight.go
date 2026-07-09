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
		return preflightShellCommand(arguments)
	}
	if err := preflightExecutable(program); err != nil {
		return err
	}
	return preflightKnownProgram(program)
}

func preflightShellCommand(arguments map[string]any) error {
	rawArgs, ok := arguments["args"].([]any)
	if !ok || len(rawArgs) < 2 {
		return nil
	}
	flag, _ := rawArgs[0].(string)
	if flag != "-c" {
		return nil
	}
	script, _ := rawArgs[1].(string)
	for _, cmd := range []string{"us-market", "feishu-send"} {
		if strings.Contains(script, cmd) {
			if err := preflightKnownProgram(cmd); err != nil {
				return err
			}
		}
	}
	return nil
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

func preflightKnownProgram(program string) error {
	base := filepath.Base(strings.TrimSpace(program))
	switch base {
	case "us-market":
		return requireEnvForProgram(base, "US_MARKET_USER_AGENT")
	case "feishu-send":
		if strings.TrimSpace(os.Getenv("FEISHU_BOT_WEBHOOK_URL")) == "" {
			return fmt.Errorf("preflight: %s requires FEISHU_BOT_WEBHOOK_URL", base)
		}
		return nil
	default:
		return nil
	}
}

func requireEnvForProgram(program string, names ...string) error {
	for _, name := range names {
		if strings.TrimSpace(os.Getenv(name)) == "" {
			return fmt.Errorf("preflight: %s requires %s", program, name)
		}
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
