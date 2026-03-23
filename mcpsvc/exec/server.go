package execmcp

import (
	"context"

	"github.com/OctoSucker/mcpsvc/internal/mcpx"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const ImplementationName = "octoplus-mcp-exec"

func NewMCPServer(cfg Config) *mcp.Server {
	eng := NewEngine(cfg)
	srv := mcp.NewServer(&mcp.Implementation{Name: ImplementationName, Version: "0.1"}, nil)
	registerExecTool(srv, eng)
	return srv
}

func registerExecTool(srv *mcp.Server, eng *Engine) {
	type runArgs struct {
		Command    string            `json:"command" jsonschema:"command line split by spaces; no shell (no pipes)"`
		WorkDir    string            `json:"work_dir,omitempty" jsonschema:"cwd inside EXEC_WORKSPACE_DIRS; default first root"`
		TimeoutSec int               `json:"timeout_sec,omitempty"`
		Env        map[string]string `json:"env,omitempty"`
	}
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "run_command",
		Description: "Run one command under EXEC_WORKSPACE_DIRS. No shell: argv = strings.Fields(command). rm is redirected to workspace .trash. Returns stdout, stderr, exit_code. If the user asked to send command output via Telegram, use send_telegram_message with the stdout text.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args runArgs) (*mcp.CallToolResult, any, error) {
		if args.Command == "" {
			return mcpx.TextResult(mcpx.JSONText(map[string]any{"success": false, "error": "command is required"})), nil, nil
		}
		out, err := eng.RunCommand(ctx, args.Command, args.WorkDir, args.TimeoutSec, args.Env)
		if err != nil {
			return mcpx.TextResult(mcpx.JSONText(map[string]any{"success": false, "error": err.Error()})), nil, nil
		}
		return mcpx.TextResult(mcpx.JSONText(out)), nil, nil
	})
}
