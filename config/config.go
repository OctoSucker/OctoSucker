package config

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/google-agentic-commerce/a2a-x402/core/types"
)

// AgentConfig 包含 Agent 的完整配置
type AgentConfig struct {
	Port string `json:"port"` // 服务端口，如 ":8080"

	Merchant        *Merchant              `json:"merchant"`
	NetworkKeyPairs []types.NetworkKeyPair `json:"networkKeyPairs"`
	LLM             *LLMConfig             `json:"llm,omitempty"`
	Personality     string                 `json:"personality,omitempty"`       // Agent 性格
	ToolsConfigPath string                 `json:"tools_config_path,omitempty"` // 工具配置文件路径，如 "config/tools.json" 或 "config/tools/"
}

type Merchant struct {
	Name           string                `json:"name"`
	Description    string                `json:"description"`
	URL            string                `json:"url"`            // Agent 的完整 URL，如 "http://localhost:8080"
	FacilitatorURL string                `json:"facilitatorURL"` // Facilitator URL
	NetworkConfigs []types.NetworkConfig `json:"networkConfigs"`
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
