package execbuiltin

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/OctoSucker/agent/internal/config"
	"github.com/OctoSucker/agent/pkg/ports"
)

const macOSSandboxExecBinary = "/usr/bin/sandbox-exec"

func configureMacOSRunner(rc *runnerConfig, execCfg config.Exec) error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("exec builtin: backend %q is only supported on darwin (host is %s)", config.ExecBackendMacOSSandboxExec, runtime.GOOS)
	}
	if execCfg.MacOSSandboxProfile == "" {
		return fmt.Errorf("exec builtin: macos_sandbox_profile is required for backend %q", config.ExecBackendMacOSSandboxExec)
	}
	st, err := os.Stat(macOSSandboxExecBinary)
	if err != nil {
		return fmt.Errorf("exec builtin: %s: %w", macOSSandboxExecBinary, err)
	}
	if st.IsDir() {
		return fmt.Errorf("exec builtin: %s is not a binary", macOSSandboxExecBinary)
	}
	rc.macOSSandboxProfile = execCfg.MacOSSandboxProfile
	rc.sandboxExecPath = macOSSandboxExecBinary
	return nil
}

func (r *Runner) runMacOSSandbox(ctx context.Context, wd, tool string, argv []string, args map[string]any) (ports.ToolResult, error) {
	sbxArgs := []string{"-f", r.cfg.macOSSandboxProfile, "-D", "WORKSPACE=" + wd}
	for i, root := range r.cfg.roots {
		sbxArgs = append(sbxArgs, "-D", fmt.Sprintf("ROOT%d=%s", i, root))
	}
	sbxArgs = append(sbxArgs, "--")
	sbxArgs = append(sbxArgs, argv...)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, r.cfg.sandboxExecPath, sbxArgs...)
	cmd.Dir = wd
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if rawEnv, ok := args["env"]; ok && rawEnv != nil {
		envMap, ok := rawEnv.(map[string]any)
		if !ok {
			return ports.ToolResult{Err: fmt.Errorf("exec builtin: argument \"env\" must be object")}, fmt.Errorf("exec builtin: argument \"env\" must be object")
		}
		extra := make(map[string]string, len(envMap))
		for k, v := range envMap {
			vs, ok := v.(string)
			if !ok {
				return ports.ToolResult{Err: fmt.Errorf("exec builtin: env value for %q must be string", k)}, fmt.Errorf("exec builtin: env value for %q must be string", k)
			}
			extra[k] = vs
		}
		cmd.Env = overlayEnv(os.Environ(), extra)
	}
	err := cmd.Run()
	if err != nil {
		runErr := formatExecRunError(tool, err, stderr.String())
		return ports.ToolResult{Err: runErr}, runErr
	}
	return ports.ToolResult{
		Output: map[string]any{
			"stdout":    stdout.String(),
			"stderr":    stderr.String(),
			"exit_code": 0,
			"work_dir":  wd,
		},
	}, nil
}

func overlayEnv(base []string, extra map[string]string) []string {
	if len(extra) == 0 {
		return append([]string(nil), base...)
	}
	skip := make(map[string]struct{}, len(extra))
	for k := range extra {
		skip[k] = struct{}{}
	}
	out := make([]string, 0, len(base)+len(extra))
	for _, pair := range base {
		k, _, ok := strings.Cut(pair, "=")
		if !ok || k == "" {
			out = append(out, pair)
			continue
		}
		if _, drop := skip[k]; drop {
			continue
		}
		out = append(out, pair)
	}
	for k, v := range extra {
		out = append(out, k+"="+v)
	}
	return out
}
