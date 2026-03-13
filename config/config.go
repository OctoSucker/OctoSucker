package config

import (
	"encoding/json"
	"fmt"
	"os"
)

type AgentConfig struct {
	LLM    *LLMConfig                        `json:"llm,omitempty"`
	ReAct  *ReActConfig                      `json:"react,omitempty"`
	ToolProviders map[string]map[string]interface{} `json:"tool_providers,omitempty"`
}

type ReActConfig struct {
	MaxReActIterations int    `json:"max_react_iterations,omitempty"`
	TaskTimeoutSec     int    `json:"task_timeout_sec,omitempty"`
	MemoryPath         string `json:"memory_path,omitempty"`
	EmbeddingModel     string `json:"embedding_model,omitempty"`
}

type LLMConfig struct {
	BaseURL string `json:"baseURL"`
	APIKey  string `json:"apiKey"`
	Model   string `json:"model"`
}

func LoadConfig(configPath string) (*LLMConfig, *ReActConfig, map[string]map[string]interface{}, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg AgentConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, nil, nil, fmt.Errorf("failed to parse config file: %w", err)
	}
	return cfg.LLM, cfg.ReAct, cfg.ToolProviders, nil
}
