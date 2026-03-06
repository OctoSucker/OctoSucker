package config

import (
	"encoding/json"
	"fmt"
	"os"
)

type AgentConfig struct {
	LLM *LLMConfig `json:"llm,omitempty"`

	// Skill 配置（可选）
	Telegram *TelegramConfig `json:"telegram,omitempty"` // Telegram Skill 配置
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
