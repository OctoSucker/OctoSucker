package llmclient

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	openai "github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/shared"
)

type OpenAI struct {
	Client         openai.Client
	Model          string
	EmbeddingModel string
}

func NewOpenAI(baseURL, apiKey, model, embeddingModel string) *OpenAI {
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
	}
}

func (c *OpenAI) Complete(ctx context.Context, system string, user string) (string, error) {
	if c.Model == "" {
		return "", fmt.Errorf("llmclient.OpenAI: model required")
	}
	m := shared.ChatModel(c.Model)
	res, err := c.Client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Model: m,
		Messages: []openai.ChatCompletionMessageParamUnion{
			{OfSystem: &openai.ChatCompletionSystemMessageParam{
				Content: openai.ChatCompletionSystemMessageParamContentUnion{
					OfString: openai.String(system),
				},
			}},
			{OfUser: &openai.ChatCompletionUserMessageParam{
				Content: openai.ChatCompletionUserMessageParamContentUnion{
					OfString: openai.String(user),
				},
			}},
		},
	})
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
	raw, err := c.Complete(ctx, system, user)
	if err != nil {
		return err
	}
	content := strings.TrimSpace(raw)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)
	if err := json.Unmarshal([]byte(content), out); err != nil {
		return fmt.Errorf("llmclient.OpenAI: unmarshal completion json: %w", err)
	}
	return nil
}

func (c *OpenAI) Embed(ctx context.Context, text string) ([]float32, error) {
	if c.EmbeddingModel == "" {
		return nil, fmt.Errorf("llmclient.OpenAI: embedding model required")
	}
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
