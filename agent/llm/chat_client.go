package llm

import (
	"context"

	"github.com/openai/openai-go"
)

type ToolChatClient interface {
	ChatCompletionWithTools(
		ctx context.Context,
		messages []openai.ChatCompletionMessageParamUnion,
		tools []map[string]interface{},
	) (*ChatCompletionResult, error)
}
