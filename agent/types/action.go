package types

// Action 表示 Agent 的决策动作类型
type Action string

const (
	ActionSendRequest Action = "send_request" // 发送请求
	ActionSendChat    Action = "send_chat"    // 发送聊天消息
	ActionWait        Action = "wait"         // 等待
	ActionAnalyze     Action = "analyze"      // 分析
)

// String 返回 Action 的字符串表示
func (a Action) String() string {
	return string(a)
}

// IsValid 检查 Action 是否有效
func (a Action) IsValid() bool {
	return a == ActionSendRequest || a == ActionSendChat || a == ActionWait || a == ActionAnalyze
}
