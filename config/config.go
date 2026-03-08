package config

import (
	"encoding/json"
	"fmt"
	"os"
)

type AgentConfig struct {
	LLM *LLMConfig `json:"llm,omitempty"`

	// Agent/ReAct 配置（可选）
	Agent *ReActConfig `json:"agent,omitempty"` // ReAct 循环与任务超时

	// Skill 配置（可选）
	Telegram *TelegramConfig `json:"telegram,omitempty"` // Telegram Skill 配置
	Fs       *FsConfig      `json:"fs,omitempty"`       // 文件系统 Skill 配置
	Web      *WebConfig     `json:"web,omitempty"`      // Web Skill 配置
}

// ReActConfig ReAct 循环与单任务超时配置
type ReActConfig struct {
	MaxReActIterations int `json:"max_react_iterations,omitempty"` // 最大 ReAct 迭代次数，默认 20
	TaskTimeoutSec     int `json:"task_timeout_sec,omitempty"`      // 单任务超时秒数，0 表示不限制
}

// WebConfig Web Skill 配置（浏览器自动化 + HTTP 抓取，无 Brave/Serper）
type WebConfig struct {
	FetchMaxChars int `json:"fetch_max_chars,omitempty"` // HTTP 抓取与 extract 最大字符数，默认 50000
}

// FsConfig 文件系统 Skill 配置
type FsConfig struct {
	WorkspaceDirs []string `json:"workspace_dirs,omitempty"` // 允许访问的根目录列表（相对或绝对路径）
}

// TelegramConfig Telegram Skill 配置
type TelegramConfig struct {
	BotToken        string  `json:"bot_token"`                   // Bot Token（必需）
	DMPolicy        string  `json:"dm_policy,omitempty"`         // DM 策略：disabled, open, allowlist, pairing
	GroupPolicy     string  `json:"group_policy,omitempty"`      // 群组策略：disabled, allowlist, open
	RequireMention  bool    `json:"require_mention,omitempty"`   // 是否需要在群组中 @mention bot
	AllowedChatIDs  []int64 `json:"allowed_chat_ids,omitempty"`  // 允许的聊天 ID 列表
	AllowedUserIDs  []int64 `json:"allowed_user_ids,omitempty"`  // 允许的用户 ID 列表
	MaxMessageStore int     `json:"max_message_store,omitempty"` // 最大消息存储数量
}

// LLMConfig LLM 配置
type LLMConfig struct {
	BaseURL string `json:"baseURL"` // LLM API 基础 URL
	APIKey  string `json:"apiKey"`  // LLM API Key
	Model   string `json:"model"`   // 模型名称，如 "deepseek-v3.2-exp"
}

func LoadConfig(configPath string) (*AgentConfig, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config AgentConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return &config, nil
}
