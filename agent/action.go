package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/OctoSucker/octosucker/agent/llm"
)

// executeToolCall 行动阶段：执行工具调用
func (a *Agent) executeToolCall(ctx context.Context, toolCall llm.ToolCall) (interface{}, error) {
	// 将参数转换为 JSON 字符串
	log.Printf("Tool %s arguments: %v", toolCall.Name, toolCall.Arguments)
	argumentsJSON, err := json.Marshal(toolCall.Arguments)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal tool arguments: %w", err)
	}

	// 执行工具
	toolResult, err := a.toolRegistry.ExecuteTool(toolCall.Name, string(argumentsJSON))
	if err != nil {
		log.Printf("Tool %s execution failed: %v", toolCall.Name, err)
		return nil, err
	}

	log.Printf("Tool %s executed successfully, result: %v", toolCall.Name, toolResult)
	return toolResult, nil
}
