package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/OctoSucker/octosucker/agent/llm"
	"github.com/openai/openai-go"
)

// reasonNextAction 推理阶段：LLM 分析当前状态并决定下一步行动
func (a *Agent) reasonNextAction(ctx context.Context, session *Session, task *Task) (*llm.ChatCompletionResult, error) {
	// 构建消息列表（包含系统消息、历史消息、当前任务）
	messages := []openai.ChatCompletionMessageParamUnion{
		openai.SystemMessage(systemPrompt),
	}

	// 添加会话历史消息
	messages = append(messages, session.Messages...)

	// 只在会话第一次创建时添加任务输入（通过检查消息历史是否为空来判断）
	// 这样可以避免每次循环都重复添加任务输入，导致 LLM 认为任务还没完成
	// 注意：task.Input 已经由 Skill 或调用者构建了完整的提示词，包含所有必要信息，直接使用即可
	if task.Input != "" && len(session.Messages) == 0 {
		messages = append(messages, openai.UserMessage(task.Input))
	}

	// 获取 Tool 定义
	toolDefs := a.toolRegistry.GetAllTools()

	// 调用 LLM
	result, err := a.llmClient.ChatCompletionWithTools(ctx, messages, toolDefs)
	if err != nil {
		log.Printf("LLM reasoning: API call failed: %v", err)
		return nil, fmt.Errorf("failed to call LLM: %w", err)
	}

	// 将 LLM 的回复添加到会话历史
	// 关键问题：OpenAI 要求 tool message 前面必须有对应的 assistant message with tool_calls
	// 解决方案：我们需要保存 LLM 返回的完整 assistant message（包括 tool_calls）
	// 但由于类型限制，我们需要通过 JSON 构造符合格式的消息
	if result.HasToolCalls && len(result.ToolCalls) > 0 {
		// 转换为 OpenAI 格式的 tool calls
		toolCalls := make([]map[string]interface{}, 0, len(result.ToolCalls))
		for _, tc := range result.ToolCalls {
			argsJSON, _ := json.Marshal(tc.Arguments)
			toolCalls = append(toolCalls, map[string]interface{}{
				"id":   tc.ID,
				"type": "function",
				"function": map[string]interface{}{
					"name":      tc.Name,
					"arguments": string(argsJSON),
				},
			})
		}
		// 构造 assistant message with tool_calls
		assistantMsgData := map[string]interface{}{
			"role":       "assistant",
			"content":    nil,
			"tool_calls": toolCalls,
		}
		assistantMsgBytes, _ := json.Marshal(assistantMsgData)
		var assistantMsg openai.ChatCompletionMessageParamUnion
		if err := json.Unmarshal(assistantMsgBytes, &assistantMsg); err != nil {
			log.Printf("Failed to create assistant message with tool calls: %v", err)
		} else {
			session.Messages = append(session.Messages, assistantMsg)
			log.Printf("Added assistant message with %d tool calls to session history", len(toolCalls))
		}
	} else if result.Content != "" {
		// 添加 assistant message with content
		session.Messages = append(session.Messages, openai.AssistantMessage(result.Content))
		log.Printf("Added assistant message with content to session history")
	}

	return result, nil
}
