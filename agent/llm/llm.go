package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

type LLMClient struct {
	openai.Client
	model          string
	embeddingModel string
}

func NewLLMClient(baseURL, apiKey, model, embeddingModel string) *LLMClient {
	client := openai.NewClient(
		option.WithAPIKey(apiKey),
		option.WithBaseURL(baseURL),
	)
	return &LLMClient{
		Client:         client,
		model:          model,
		embeddingModel: embeddingModel,
	}
}

func (c *LLMClient) ChatCompletion(ctx context.Context, messages string) (string, error) {
	chatCompletion, err := c.Client.Chat.Completions.New(
		ctx, openai.ChatCompletionNewParams{
			Messages: []openai.ChatCompletionMessageParamUnion{
				openai.UserMessage(messages),
			},
			Model: c.model,
		},
	)
	if err != nil {
		return "", fmt.Errorf("failed to get chat completion: %w", err)
	}
	if len(chatCompletion.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}
	return chatCompletion.Choices[0].Message.Content, nil
}

func (c *LLMClient) ChatCompletionWithTools(
	ctx context.Context,
	messages []openai.ChatCompletionMessageParamUnion,
	tools []map[string]interface{},
) (*ChatCompletionResult, error) {
	params := openai.ChatCompletionNewParams{
		Messages: messages,
		Model:    c.model,
	}
	if len(tools) > 0 {
		toolParams := make([]openai.ChatCompletionToolParam, 0, len(tools))
		for _, tool := range tools {
			toolBytes, _ := json.Marshal(tool)
			var toolParam openai.ChatCompletionToolParam
			if err := json.Unmarshal(toolBytes, &toolParam); err == nil {
				toolParams = append(toolParams, toolParam)
			} else {
				log.Printf("[llm] unmarshal tool param failed: %v", err)
			}
		}
		if len(toolParams) > 0 {
			params.Tools = toolParams
		}
	}

	chatCompletion, err := c.Client.Chat.Completions.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("failed to get chat completion: %w", err)
	}

	if len(chatCompletion.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}

	choice := chatCompletion.Choices[0]
	result := &ChatCompletionResult{
		Content: choice.Message.Content,
	}
	if len(choice.Message.ToolCalls) > 0 {
		result.ToolCalls = make([]ToolCall, 0, len(choice.Message.ToolCalls))

		for _, msgToolCall := range choice.Message.ToolCalls {
			toolCall := ToolCall{
				ID:   msgToolCall.ID,
				Name: msgToolCall.Function.Name,
			}
			if msgToolCall.Function.Arguments != "" {
				if err := json.Unmarshal([]byte(msgToolCall.Function.Arguments), &toolCall.Arguments); err != nil {
					return nil, fmt.Errorf("failed to parse tool arguments: %w", err)
				}
			}

			result.ToolCalls = append(result.ToolCalls, toolCall)
		}
	}

	return result, nil
}

func (c *LLMClient) Embed(ctx context.Context, text string) ([]float32, error) {
	if text == "" {
		return nil, fmt.Errorf("empty text for embedding")
	}
	if c.embeddingModel == "" {
		return nil, fmt.Errorf("embedding_model not configured: add \"embedding_model\" under react in config (e.g. text-embedding-v3 for DashScope)")
	}
	params := openai.EmbeddingNewParams{
		Input: openai.EmbeddingNewParamsInputUnion{
			OfString: openai.String(text),
		},
		Model: openai.EmbeddingModel(c.embeddingModel),
	}
	resp, err := c.Client.Embeddings.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("failed to get embedding: %w", err)
	}
	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("no embedding data in response")
	}
	src := resp.Data[0].Embedding
	if len(src) == 0 {
		return nil, fmt.Errorf("empty embedding in response")
	}
	dst := make([]float32, len(src))
	for i, v := range src {
		dst[i] = float32(v)
	}
	return dst, nil
}

func CosineSimilarity(a, b []float32) float32 {
	if len(a) == 0 || len(b) == 0 || len(a) != len(b) {
		return 0
	}
	var dot float64
	var na float64
	var nb float64
	for i := 0; i < len(a); i++ {
		ai := float64(a[i])
		bi := float64(b[i])
		dot += ai * bi
		na += ai * ai
		nb += bi * bi
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return float32(dot / (math.Sqrt(na) * math.Sqrt(nb)))
}

type ChatCompletionResult struct {
	Content   string
	ToolCalls []ToolCall
}

func (r *ChatCompletionResult) HasToolCalls() bool {
	return len(r.ToolCalls) > 0
}

type ToolCall struct {
	ID        string
	Name      string
	Arguments map[string]interface{}
}
