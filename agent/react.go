package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/OctoSucker/octosucker/agent/llm"
	"github.com/OctoSucker/octosucker/agent/memory"
	"github.com/openai/openai-go"
)

func (a *Agent) runReActLoop(ctx context.Context, task *Task) {

	var messages []openai.ChatCompletionMessageParamUnion

	for iterations := 0; iterations < a.maxReActIterations; iterations++ {
		select {
		case <-ctx.Done():
			log.Printf("[agent] ReAct cancelled: task=%s err=%v", task.ID, ctx.Err())
			return
		default:
		}

		// 只在第一轮注入记忆
		memorySummary := ""
		if iterations == 0 {
			memorySummary = memory.BuildSummaryForQuery(ctx, a.memory, task.Input, 5, log.Printf)
		}
		result, err := a.reasonNextAction(ctx, &messages, task, memorySummary)
		if err != nil {
			log.Printf("[agent] reasoning failed: task=%s err=%v", task.ID, err)
			return
		}
		if !result.HasToolCalls() {
			if result.Content != "" {
				log.Printf("[agent] Task %s final reply: %s", task.ID, result.Content)
				if addErr := a.memory.Add(ctx, task.ID, memory.FormatTaskAnswer(task.Input, result.Content)); addErr != nil {
					log.Printf("[agent] failed to add memory: task=%s err=%v", task.ID, addErr)
				}
			}
			return
		}
		for _, toolCall := range result.ToolCalls {
			argsPreview, _ := json.Marshal(toolCall.Arguments)
			toolResult, toolErr := a.executeToolCall(ctx, toolCall)
			log.Printf("[agent] tool %s : args=%v result=%v err=%+v", toolCall.Name, toolCall.Arguments, toolResult, toolErr)

			text := memory.FormatToolStep(task.Input, toolCall.Name, string(argsPreview), fmt.Sprint(toolResult), toolErr)
			if addErr := a.memory.Add(ctx, task.ID, text); addErr != nil {
				log.Printf("[agent] failed to add memory: task=%s err=%v", task.ID, addErr)
			}
			appendToolResult(&messages, toolCall, toolResult, toolErr)
		}
	}
}

func (a *Agent) reasonNextAction(ctx context.Context, messages *[]openai.ChatCompletionMessageParamUnion, task *Task, memorySummary string) (*llm.ChatCompletionResult, error) {
	full := []openai.ChatCompletionMessageParamUnion{
		openai.SystemMessage(a.systemPrompt),
	}
	if memorySummary != "" {
		full = append(full, openai.SystemMessage("下面是与你当前任务可能相关的历史记忆（向量检索得到）：\n"+memorySummary))
	}
	full = append(full, *messages...)
	if task.Input != "" && len(*messages) == 0 {
		full = append(full, openai.UserMessage(task.Input))
	}

	toolDefs := a.toolRegistry.GetAllTools()
	result, err := a.llmClient.ChatCompletionWithTools(ctx, full, toolDefs)
	if err != nil {
		log.Printf("[agent] LLM call failed: %v", err)
		return nil, fmt.Errorf("failed to call LLM: %w", err)
	}

	if result.HasToolCalls() {
		toolCalls := make([]map[string]interface{}, 0, len(result.ToolCalls))
		for _, toolCall := range result.ToolCalls {
			argsJSON, _ := json.Marshal(toolCall.Arguments)
			toolCalls = append(toolCalls, map[string]interface{}{
				"id":   toolCall.ID,
				"type": "function",
				"function": map[string]interface{}{
					"name":      toolCall.Name,
					"arguments": string(argsJSON),
				},
			})
		}
		assistantMsgData := map[string]interface{}{
			"role":       "assistant",
			"content":    nil,
			"tool_calls": toolCalls,
		}
		assistantMsgBytes, _ := json.Marshal(assistantMsgData)
		var assistantMsg openai.ChatCompletionMessageParamUnion
		if err := json.Unmarshal(assistantMsgBytes, &assistantMsg); err != nil {
			log.Printf("[agent] failed to build assistant message with tool_calls: %v", err)
		} else {
			*messages = append(*messages, assistantMsg)
		}
	} else if result.Content != "" {
		*messages = append(*messages, openai.AssistantMessage(result.Content))
	}

	return result, nil
}

func (a *Agent) executeToolCall(ctx context.Context, toolCall llm.ToolCall) (interface{}, error) {
	argumentsJSON, err := json.Marshal(toolCall.Arguments)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal tool arguments: %w", err)
	}
	toolResult, err := a.toolRegistry.ExecuteTool(ctx, toolCall.Name, string(argumentsJSON))
	if err != nil {
		log.Printf("[agent] tool %s execution failed: %v", toolCall.Name, err)
		return nil, err
	}
	return toolResult, nil
}

func appendToolResult(messages *[]openai.ChatCompletionMessageParamUnion, toolCall llm.ToolCall, result interface{}, err error) {
	var toolResultJSON []byte
	if err != nil {
		errorResult := map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		}
		toolResultJSON, _ = json.Marshal(errorResult)
	} else {
		toolResultJSON, _ = json.Marshal(result)
	}
	const maxToolResultChars = 8000
	s := string(toolResultJSON)
	if len(s) > maxToolResultChars {
		s = s[:maxToolResultChars] + `"...(truncated)"`
	}
	*messages = append(*messages, openai.ToolMessage(s, toolCall.ID))
}
