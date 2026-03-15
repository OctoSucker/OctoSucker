package runtime

import (
	"encoding/json"

	"github.com/openai/openai-go"
)

const maxToolResultChars = 8000

type TaskMemory struct {
	messages []openai.ChatCompletionMessageParamUnion
}

func NewTaskMemory(initialUserMessage string) *TaskMemory {
	m := &TaskMemory{}
	if initialUserMessage != "" {
		m.messages = []openai.ChatCompletionMessageParamUnion{openai.UserMessage(initialUserMessage)}
	}
	return m
}

func (m *TaskMemory) AddUserMessage(content string) {
	if content == "" {
		return
	}
	m.messages = append(m.messages, openai.UserMessage(content))
}

func (m *TaskMemory) AddAssistantMessage(content string) {
	m.messages = append(m.messages, openai.AssistantMessage(content))
}

func (m *TaskMemory) AddAssistantToolCalls(toolCalls []map[string]interface{}) {
	assistantMsgData := map[string]interface{}{
		"role":       "assistant",
		"content":    nil,
		"tool_calls": toolCalls,
	}
	assistantMsgBytes, _ := json.Marshal(assistantMsgData)
	var msg openai.ChatCompletionMessageParamUnion
	_ = json.Unmarshal(assistantMsgBytes, &msg)
	m.messages = append(m.messages, msg)
}

func (m *TaskMemory) AddToolResult(toolCallID, result string) {
	if len(result) > maxToolResultChars {
		result = result[:maxToolResultChars] + `"...(truncated)"`
	}
	m.messages = append(m.messages, openai.ToolMessage(result, toolCallID))
}

func (m *TaskMemory) Messages() []openai.ChatCompletionMessageParamUnion {
	return m.messages
}
