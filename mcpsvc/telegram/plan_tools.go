package telegram

import (
	"context"

	"github.com/OctoSucker/mcpsvc/internal/mcpx"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerPlanTools(srv *mcp.Server) {
	type echoArgs struct {
		Text string `json:"text" jsonschema:"text to echo"`
	}
	mcp.AddTool(srv, &mcp.Tool{Name: "echo", Description: "Echo input text."}, func(ctx context.Context, _ *mcp.CallToolRequest, args echoArgs) (*mcp.CallToolResult, any, error) {
		return mcpx.TextResult(args.Text), nil, nil
	})

	mcp.AddTool(srv, &mcp.Tool{Name: "finish", Description: "Mark the task as finished."}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
		return mcpx.TextResult("done"), nil, nil
	})
}
