package mcpclient

import (
	"errors"
	"fmt"
	"strings"

	"github.com/OctoSucker/agent/pkg/ports"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func mcpResultToPorts(res *mcp.CallToolResult) ports.ToolResult {
	if res == nil {
		return ports.ToolResult{OK: false, Err: errors.New("nil tool result")}
	}
	if res.IsError {
		msg := joinTextContent(res.Content)
		if msg == "" {
			msg = "tool error"
		}
		return ports.ToolResult{OK: false, Output: msg, Err: errors.New(msg)}
	}
	if res.StructuredContent != nil {
		return ports.ToolResult{OK: true, Output: res.StructuredContent}
	}
	text := joinTextContent(res.Content)
	return ports.ToolResult{OK: true, Output: text}
}

func joinTextContent(content []mcp.Content) string {
	var parts []string
	for _, c := range content {
		if tc, ok := c.(*mcp.TextContent); ok {
			parts = append(parts, tc.Text)
		} else {
			parts = append(parts, fmt.Sprint(c))
		}
	}
	return strings.Join(parts, "\n")
}
