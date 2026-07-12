package llmclient

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	openai "github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/shared"
)

type OpenAI struct {
	Client         openai.Client
	Model          string
	EmbeddingModel string
	EnableThinking *bool
}

const (
	defaultCompleteTimeout = 45 * time.Second
	defaultEmbedTimeout    = 20 * time.Second
)

func NewOpenAI(baseURL, apiKey, model, embeddingModel string, enableThinking *bool) *OpenAI {
	var opts []option.RequestOption
	if baseURL != "" {
		opts = append(opts, option.WithBaseURL(baseURL))
	}
	if apiKey != "" {
		opts = append(opts, option.WithAPIKey(apiKey))
	}
	return &OpenAI{
		Client:         openai.NewClient(opts...),
		Model:          model,
		EmbeddingModel: embeddingModel,
		EnableThinking: enableThinking,
	}
}

func (c *OpenAI) Complete(ctx context.Context, systemMsg string, userMsg string) (string, error) {
	return c.complete(ctx, systemMsg, userMsg, false)
}

func (c *OpenAI) complete(ctx context.Context, systemMsg, userMsg string, jsonObject bool) (string, error) {
	if c.Model == "" {
		return "", fmt.Errorf("llmclient.OpenAI: model required")
	}
	ctx, cancel := withDefaultTimeout(ctx, defaultCompleteTimeout)
	defer cancel()

	m := shared.ChatModel(c.Model)
	params := openai.ChatCompletionNewParams{
		Model: m,
		Messages: []openai.ChatCompletionMessageParamUnion{
			{OfSystem: &openai.ChatCompletionSystemMessageParam{
				Content: openai.ChatCompletionSystemMessageParamContentUnion{
					OfString: openai.String(systemMsg),
				},
			}},
			{OfUser: &openai.ChatCompletionUserMessageParam{
				Content: openai.ChatCompletionUserMessageParamContentUnion{
					OfString: openai.String(userMsg),
				},
			}},
		},
	}
	if jsonObject {
		format := shared.NewResponseFormatJSONObjectParam()
		params.ResponseFormat = openai.ChatCompletionNewParamsResponseFormatUnion{OfJSONObject: &format}
	}
	var requestOptions []option.RequestOption
	if c.EnableThinking != nil {
		requestOptions = append(requestOptions, option.WithJSONSet("enable_thinking", *c.EnableThinking))
	}
	res, err := c.Client.Chat.Completions.New(ctx, params, requestOptions...)
	if err != nil {
		return "", err
	}
	if len(res.Choices) == 0 {
		return "", fmt.Errorf("llmclient.OpenAI: empty completion choices")
	}
	return res.Choices[0].Message.Content, nil
}

func (c *OpenAI) CompleteJSON(ctx context.Context, system string, user string, out any) error {
	if out == nil {
		return fmt.Errorf("llmclient.OpenAI: out is nil")
	}
	raw, err := c.complete(ctx, system, user, true)
	if err != nil {
		return err
	}
	content := normalizeJSONCompletion(raw)
	if err := json.Unmarshal([]byte(content), out); err != nil {
		return fmt.Errorf("llmclient.OpenAI: unmarshal completion json: %w", err)
	}
	return nil
}

func normalizeJSONCompletion(raw string) string {
	content := strings.TrimSpace(raw)
	if strings.HasPrefix(content, "```") {
		if i := strings.Index(content, "\n"); i >= 0 {
			content = content[i+1:]
		}
		content = strings.TrimSpace(strings.TrimSuffix(content, "```"))
	}
	if extracted, ok := extractBalancedJSON(content); ok {
		return extracted
	}
	return strings.TrimSpace(content)
}

func extractBalancedJSON(s string) (string, bool) {
	start := -1
	for i, r := range s {
		if r == '{' || r == '[' {
			start = i
			break
		}
	}
	if start < 0 {
		return "", false
	}

	var stack []rune
	inString := false
	escaped := false
	for i, r := range s[start:] {
		if inString {
			if escaped {
				escaped = false
				continue
			}
			switch r {
			case '\\':
				escaped = true
			case '"':
				inString = false
			}
			continue
		}
		switch r {
		case '"':
			inString = true
		case '{', '[':
			stack = append(stack, r)
		case '}', ']':
			if len(stack) == 0 {
				return "", false
			}
			open := stack[len(stack)-1]
			if (open == '{' && r != '}') || (open == '[' && r != ']') {
				return "", false
			}
			stack = stack[:len(stack)-1]
			if len(stack) == 0 {
				return strings.TrimSpace(s[start : start+i+len(string(r))]), true
			}
		}
	}
	return "", false
}

func (c *OpenAI) Embed(ctx context.Context, text string) ([]float32, error) {
	if c.EmbeddingModel == "" {
		return nil, fmt.Errorf("llmclient.OpenAI: embedding model required")
	}
	ctx, cancel := withDefaultTimeout(ctx, defaultEmbedTimeout)
	defer cancel()

	model := openai.EmbeddingModel(c.EmbeddingModel)
	res, err := c.Client.Embeddings.New(ctx, openai.EmbeddingNewParams{
		Model: model,
		Input: openai.EmbeddingNewParamsInputUnion{
			OfString: openai.String(text),
		},
	})
	if err != nil {
		return nil, err
	}
	if len(res.Data) == 0 {
		return nil, fmt.Errorf("llmclient.OpenAI: empty embedding response")
	}
	emb := res.Data[0].Embedding
	out := make([]float32, len(emb))
	for i := range emb {
		out[i] = float32(emb[i])
	}
	return out, nil
}

func withDefaultTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if ctx == nil {
		return context.WithTimeout(context.Background(), timeout)
	}
	if _, ok := ctx.Deadline(); ok {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, timeout)
}
