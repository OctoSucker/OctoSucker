package execbuiltin

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"

	types "github.com/OctoSucker/octosucker/internal/runtime/model"
)

func (r *Runner) runHost(ctx context.Context, wd, tool string, argv []string, args map[string]any) (types.ToolResult, error) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = wd
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if rawEnv, ok := args["env"]; ok && rawEnv != nil {
		envMap, ok := rawEnv.(map[string]any)
		if !ok {
			return types.ToolResult{Err: fmt.Errorf("exec builtin: argument \"env\" must be object")}, fmt.Errorf("exec builtin: argument \"env\" must be object")
		}
		extra := make(map[string]string, len(envMap))
		for k, v := range envMap {
			vs, ok := v.(string)
			if !ok {
				return types.ToolResult{Err: fmt.Errorf("exec builtin: env value for %q must be string", k)}, fmt.Errorf("exec builtin: env value for %q must be string", k)
			}
			extra[k] = vs
		}
		cmd.Env = overlayEnv(os.Environ(), extra)
	}
	err := cmd.Run()
	if err != nil {
		runErr := formatExecRunError(tool, err, stderr.String())
		return types.ToolResult{Err: runErr}, runErr
	}
	return types.ToolResult{
		Output: map[string]any{
			"stdout":    stdout.String(),
			"stderr":    stderr.String(),
			"exit_code": 0,
			"work_dir":  wd,
		},
	}, nil
}
