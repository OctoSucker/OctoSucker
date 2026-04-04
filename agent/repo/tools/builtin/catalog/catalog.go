package catalog

import (
	"context"
	"fmt"
	"strings"

	"github.com/OctoSucker/agent/pkg/llmclient"
	"github.com/OctoSucker/agent/pkg/ports"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ToolNaturalLanguageReply answers the user in plain text when no other tools are needed (greetings, Q&A, small talk).
const ToolNaturalLanguageReply = "natural_language_reply"

type Runner struct {
	llm *llmclient.OpenAI
}

func NewRunner(llm *llmclient.OpenAI) (*Runner, error) {
	if llm == nil {
		return nil, fmt.Errorf("catalog builtin: llm client is required")
	}
	return &Runner{llm: llm}, nil
}

func justChatInputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"user_message": map[string]any{
				"type":        "string",
				"description": "The user's message to answer; use their wording or a concise paraphrase.",
			},
		},
		"required":             []string{"user_message"},
		"additionalProperties": false,
	}
}

const justChatSystemPrompt = `You are a helpful assistant. Reply in plain text only (no JSON, no tool calls). Match the user's language when they write in a specific language. Be concise unless the user asks for detail.`

// Name is the ToolRegistry.Backends map key for this provider (not a user-facing tool id).
func (r *Runner) Name() string { return "catalog" }

func (r *Runner) ToolList(ctx context.Context) ([]*mcp.Tool, error) {
	return []*mcp.Tool{
		{
			Name: ToolNaturalLanguageReply,
			Description: "Reply to the user in plain text. Use for greetings, small talk, or questions that need no other tools " +
				"(no shell exec, no file ops, no Telegram send_message, etc.). The runtime shows this text to the user.",
			InputSchema: justChatInputSchema(),
		},
	}, nil
}

func (r *Runner) HasTool(name string) bool {
	return name == ToolNaturalLanguageReply
}

func (r *Runner) Tool(tool string) (*mcp.Tool, error) {
	tools, err := r.ToolList(context.Background())
	if err != nil {
		return nil, err
	}
	for _, t := range tools {
		if t != nil && t.Name == tool {
			return t, nil
		}
	}
	return nil, fmt.Errorf("catalog builtin: unknown tool %q", tool)
}

func (r *Runner) Invoke(ctx context.Context, localTool string, arguments map[string]any) (ports.ToolResult, error) {
	switch localTool {
	case ToolNaturalLanguageReply:
		um, err := parseUserMessage(arguments)
		if err != nil {
			return ports.ToolResult{Err: err}, nil
		}
		reply, err := r.llm.Complete(ctx, justChatSystemPrompt, um)
		if err != nil {
			return ports.ToolResult{Err: fmt.Errorf("catalog builtin: %s: %w", ToolNaturalLanguageReply, err)}, fmt.Errorf("catalog builtin: %s: %w", ToolNaturalLanguageReply, err)
		}
		return ports.ToolResult{Output: strings.TrimSpace(reply)}, nil
	default:
		return ports.ToolResult{Err: fmt.Errorf("catalog builtin: unknown tool %q", localTool)}, fmt.Errorf("catalog builtin: unknown tool %q", localTool)
	}
}

func parseUserMessage(args map[string]any) (string, error) {
	if args == nil {
		return "", fmt.Errorf("catalog builtin: %s: arguments required", ToolNaturalLanguageReply)
	}
	raw, ok := args["user_message"]
	if !ok {
		return "", fmt.Errorf("catalog builtin: %s: user_message is required", ToolNaturalLanguageReply)
	}
	s, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("catalog builtin: %s: user_message must be a string", ToolNaturalLanguageReply)
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return "", fmt.Errorf("catalog builtin: %s: user_message must be non-empty", ToolNaturalLanguageReply)
	}
	return s, nil
}
