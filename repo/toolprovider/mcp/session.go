package mcp

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/OctoSucker/octosucker/engine/types"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// RemoteSession is an MCP client session and cached tool list for one server.
type RemoteSession struct {
	name     string
	endpoint string
	mu       sync.RWMutex
	sess     *mcp.ClientSession
	Tools    map[string]*mcp.Tool
}

func NewRemoteSession(ctx context.Context, endpoint string) (*RemoteSession, error) {
	if endpoint == "" {
		return nil, fmt.Errorf("mcp: endpoint is required")
	}
	s := &RemoteSession{
		Tools:    make(map[string]*mcp.Tool),
		endpoint: endpoint,
	}
	mc := mcp.NewClient(&mcp.Implementation{Name: "octosucker-agent", Version: "0.1"}, nil)
	sess, err := mc.Connect(
		ctx,
		&mcp.StreamableClientTransport{Endpoint: endpoint},
		nil,
	)
	if err != nil {
		return nil, err
	}
	s.sess = sess
	s.name = sess.InitializeResult().ServerInfo.Name
	return s, nil
}

func (s *RemoteSession) Name() (string, string) {
	return s.name, fmt.Sprintf("Remote MCP tools (endpoint %s).", s.endpoint)
}

func (s *RemoteSession) ToolList(ctx context.Context) ([]*mcp.Tool, error) {
	s.Tools = make(map[string]*mcp.Tool)
	var cursor string
	for {
		res, err := s.sess.ListTools(ctx, &mcp.ListToolsParams{Cursor: cursor})
		if err != nil {
			return nil, err
		}
		for _, t := range res.Tools {
			if t == nil || t.Name == "" {
				continue
			}
			s.Tools[t.Name] = t
		}
		if res.NextCursor == "" {
			break
		}
		cursor = res.NextCursor
	}
	var tools []*mcp.Tool
	for tool := range s.Tools {
		tools = append(tools, s.Tools[tool])
	}
	return tools, nil
}

func (s *RemoteSession) HasTool(tool string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.Tools) == 0 {
		s.ToolList(context.Background())
	}
	_, ok := s.Tools[tool]
	return ok
}

func (s *RemoteSession) Tool(tool string) (*mcp.Tool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.Tools) == 0 {
		s.ToolList(context.Background())
	}
	t, ok := s.Tools[tool]
	if !ok {
		return nil, fmt.Errorf("mcp: tool %q not exposed by MCP server", tool)
	}
	return t, nil
}

func (s *RemoteSession) Invoke(ctx context.Context, localTool string, arguments map[string]any) (types.ToolResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ok := s.HasTool(localTool)
	if !ok {
		return types.ToolResult{Err: fmt.Errorf("mcp: tool %q not exposed by MCP server", localTool)}, fmt.Errorf("mcp: tool %q not exposed by MCP server", localTool)
	}

	res, err := s.sess.CallTool(ctx, &mcp.CallToolParams{Name: localTool, Arguments: arguments})
	if err != nil {
		log.Printf("mcp: CallTool failed tool=%q arguments=%v err=%v", localTool, arguments, err)
		return types.ToolResult{Err: err}, err
	}
	if res.IsError {
		msg := joinTextContent(res.Content)
		if msg == "" {
			msg = "tool error"
		}
		return types.ToolResult{Output: msg, Err: errors.New(msg)}, nil
	}
	if res.StructuredContent != nil {
		return types.ToolResult{Output: fmt.Sprintf("%v", res.StructuredContent)}, nil
	}
	return types.ToolResult{Output: joinTextContent(res.Content)}, nil
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
