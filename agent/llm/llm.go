package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

// LLMClient LLM 客户端
type LLMClient struct {
	openai.Client
	model                   string
	supportsFunctionCalling *bool // 缓存是否支持 Function Calling
}

// NewLLMClient 创建新的 LLM 客户端
func NewLLMClient(baseURL, apiKey, model string) *LLMClient {
	client := openai.NewClient(
		option.WithAPIKey(apiKey),
		option.WithBaseURL(baseURL),
	)
	return &LLMClient{
		Client: client,
		model:  model,
	}
}

// ChatCompletion 发送聊天完成请求（兼容旧接口）
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

// ChatCompletionWithTools 发送带工具调用的聊天完成请求
// 返回工具调用结果或文本回复
func (c *LLMClient) ChatCompletionWithTools(
	ctx context.Context,
	messages []openai.ChatCompletionMessageParamUnion,
	tools []map[string]interface{},
) (*ChatCompletionResult, error) {
	params := openai.ChatCompletionNewParams{
		Messages: messages,
		Model:    c.model,
	}

	// 如果有工具，添加到请求中
	if len(tools) > 0 {
		// 转换为 OpenAI 格式
		toolParams := make([]openai.ChatCompletionToolParam, 0, len(tools))
		for _, tool := range tools {
			toolBytes, _ := json.Marshal(tool)
			var toolParam openai.ChatCompletionToolParam
			if err := json.Unmarshal(toolBytes, &toolParam); err == nil {
				toolParams = append(toolParams, toolParam)
			} else {
				log.Printf("LLM: failed to unmarshal tool param: %v, tool: %s", err, string(toolBytes))
			}
		}
		if len(toolParams) > 0 {
			params.Tools = toolParams
			log.Printf("LLM: sending request with %d tools", len(toolParams))
		}
	}

	// 打印消息历史用于调试
	log.Printf("LLM: sending %d messages to API", len(messages))
	// 注意：ChatCompletionMessageParamUnion 是联合类型，不能直接类型断言
	// 这里只打印消息数量，详细内容在 API 调用时由 SDK 处理

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

	// 检查是否有工具调用
	if len(choice.Message.ToolCalls) > 0 {
		result.HasToolCalls = true
		result.ToolCalls = make([]ToolCall, 0, len(choice.Message.ToolCalls))

		for _, tc := range choice.Message.ToolCalls {
			toolCall := ToolCall{
				ID:   tc.ID,
				Name: tc.Function.Name,
			}

			// 解析参数
			if tc.Function.Arguments != "" {
				if err := json.Unmarshal([]byte(tc.Function.Arguments), &toolCall.Arguments); err != nil {
					return nil, fmt.Errorf("failed to parse tool arguments: %w", err)
				}
			}

			result.ToolCalls = append(result.ToolCalls, toolCall)
		}
	}

	return result, nil
}

// ChatCompletionResult 聊天完成结果
type ChatCompletionResult struct {
	Content      string     // 文本内容（如果没有工具调用）
	HasToolCalls bool       // 是否有工具调用
	ToolCalls    []ToolCall // 工具调用列表
}

// ToolCall 工具调用
type ToolCall struct {
	ID        string
	Name      string
	Arguments map[string]interface{}
}

// SupportsFunctionCalling 检查模型是否支持 Function Calling
// 通过尝试一次简单的工具调用来检测
func (c *LLMClient) SupportsFunctionCalling(ctx context.Context) bool {
	if c.supportsFunctionCalling != nil {
		return *c.supportsFunctionCalling
	}

	// 尝试一个简单的工具调用
	testTools := []map[string]interface{}{
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "test_tool",
				"description": "Test tool",
				"parameters": map[string]interface{}{
					"type":       "object",
					"properties": map[string]interface{}{},
				},
			},
		},
	}

	messages := []openai.ChatCompletionMessageParamUnion{
		openai.UserMessage("请调用 test_tool"),
	}

	result, err := c.ChatCompletionWithTools(ctx, messages, testTools)
	if err != nil {
		supports := false
		c.supportsFunctionCalling = &supports
		return false
	}

	supports := result.HasToolCalls
	c.supportsFunctionCalling = &supports
	return supports
}
