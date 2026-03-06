package agent

import (
	"time"

	"github.com/OctoSucker/octosucker/agent/llm"
	"github.com/openai/openai-go"
)

// Task 表示一个任务（来自外部事件或定时触发）
type Task struct {
	ID        string    // 任务 ID
	Input     string    // 任务输入（如用户消息，由 Skill 或调用者构建完整提示词）
	CreatedAt time.Time // 创建时间
}

// Session 表示一个会话（用于维护消息历史和状态）
type Session struct {
	ID            string                                   // 会话 ID
	Messages      []openai.ChatCompletionMessageParamUnion // 消息历史
	ToolCalls     []ToolCallRecord                         // 工具调用历史
	CreatedAt     time.Time                                // 创建时间
	LastActiveAt  time.Time                                // 最后活跃时间
	MaxIterations int                                      // 最大迭代次数（防止无限循环）
}

// ToolCallRecord 记录一次工具调用
type ToolCallRecord struct {
	ToolCall  llm.ToolCall
	Result    interface{}
	Error     error
	Timestamp time.Time
}
