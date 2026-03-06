package agent

import (
	"encoding/json"
	"time"

	"github.com/OctoSucker/octosucker/agent/llm"
	"github.com/openai/openai-go"
)

// getOrCreateSession 获取或创建会话
func (a *Agent) getOrCreateSession(sessionID string) *Session {
	a.sessionsMu.RLock()
	session, exists := a.sessions[sessionID]
	a.sessionsMu.RUnlock()

	if exists {
		return session
	}

	// 创建新会话
	session = &Session{
		ID:            sessionID,
		Messages:      make([]openai.ChatCompletionMessageParamUnion, 0),
		ToolCalls:     make([]ToolCallRecord, 0),
		CreatedAt:     time.Now(),
		LastActiveAt:  time.Now(),
		MaxIterations: a.maxReActIterations,
	}

	a.sessionsMu.Lock()
	a.sessions[sessionID] = session
	a.sessionsMu.Unlock()

	return session
}

// AddToolCall 添加工具调用记录到会话
func (s *Session) AddToolCall(toolCall llm.ToolCall, result interface{}, err error) {
	// 将工具调用添加到消息历史（作为 assistant message with tool calls）
	// 注意：这里简化处理，实际应该按照 OpenAI 格式添加

	// 将工具执行结果添加到消息历史（作为 tool message）
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

	toolMessage := openai.ToolMessage(string(toolResultJSON), toolCall.ID)
	s.Messages = append(s.Messages, toolMessage)

	// 记录工具调用
	s.ToolCalls = append(s.ToolCalls, ToolCallRecord{
		ToolCall:  toolCall,
		Result:    result,
		Error:     err,
		Timestamp: time.Now(),
	})
}
