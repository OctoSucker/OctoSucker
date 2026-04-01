package catalog

import (
	"context"
	"fmt"
	"strings"

	"github.com/OctoSucker/agent/pkg/llmclient"
	"github.com/OctoSucker/agent/pkg/ports"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	CapabilityName       = "catalog"
	ToolJustChatUsingLLM = "just_chat_using_llm"
)

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

func (r *Runner) Name() string { return CapabilityName }

func (r *Runner) ToolList(ctx context.Context) ([]*mcp.Tool, error) {
	return []*mcp.Tool{
		{
			Name: ToolJustChatUsingLLM,
			Description: "Generate a natural-language reply to the user via the model. Use for greetings, small talk, or any request that needs no other tools " +
				"(no exec, no file ops, no Telegram send_message, etc.). The runtime surfaces this text to the user; do not call messaging tools only to echo chat.",
			InputSchema: justChatInputSchema(),
		},
	}, nil
}

func (r *Runner) HasTool(name string) bool {
	return name == ToolJustChatUsingLLM
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

func (r *Runner) Invoke(ctx context.Context, inv ports.CapabilityInvocation) (ports.ToolResult, error) {
	switch inv.Tool {
	case ToolJustChatUsingLLM:
		um, err := parseUserMessage(inv.Arguments)
		if err != nil {
			return ports.ToolResult{Err: err}, nil
		}
		reply, err := r.llm.Complete(ctx, justChatSystemPrompt, um)
		if err != nil {
			return ports.ToolResult{Err: fmt.Errorf("catalog builtin: just_chat_using_llm: %w", err)}, fmt.Errorf("catalog builtin: just_chat_using_llm: %w", err)
		}
		return ports.ToolResult{Output: strings.TrimSpace(reply)}, nil
	default:
		return ports.ToolResult{Err: fmt.Errorf("catalog builtin: unknown tool %q", inv.Tool)}, fmt.Errorf("catalog builtin: unknown tool %q", inv.Tool)
	}
}

func parseUserMessage(args map[string]any) (string, error) {
	if args == nil {
		return "", fmt.Errorf("catalog builtin: just_chat_using_llm: arguments required")
	}
	raw, ok := args["user_message"]
	if !ok {
		return "", fmt.Errorf("catalog builtin: just_chat_using_llm: user_message is required")
	}
	s, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("catalog builtin: just_chat_using_llm: user_message must be a string")
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return "", fmt.Errorf("catalog builtin: just_chat_using_llm: user_message must be non-empty")
	}
	return s, nil
}
