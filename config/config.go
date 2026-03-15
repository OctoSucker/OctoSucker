package config

import (
	"encoding/json"
	"fmt"
	"os"
)

type AgentConfig struct {
	LLM                 *LLMConfig                        `json:"llm,omitempty"`
	ReAct               *ReActConfig                     `json:"react,omitempty"`
	ToolProviders       map[string]map[string]interface{} `json:"tool_providers,omitempty"`
	McpServers []map[string]interface{} `json:"mcp_servers,omitempty"`
}

type ReActConfig struct {
	MaxReActIterations int    `json:"max_react_iterations,omitempty"`
	TaskTimeoutSec     int    `json:"task_timeout_sec,omitempty"`
	MaxToolRetries     int    `json:"max_tool_retries,omitempty"`
	ToolTimeoutMs      int    `json:"tool_timeout_ms,omitempty"`
	SelectorSemanticCandidateLimit int `json:"selector_semantic_candidate_limit,omitempty"`
	SelectorEmbeddingCacheTTLSec   int `json:"selector_embedding_cache_ttl_sec,omitempty"`
	WorkflowBindingCachePath       string `json:"workflow_binding_cache_path,omitempty"`
	MemoryPath         string `json:"memory_path,omitempty"`
	EmbeddingModel     string `json:"embedding_model,omitempty"`
	SkillWorkflows     []SkillWorkflowTemplate `json:"skill_workflows,omitempty"`
}

type SkillWorkflowTemplate struct {
	Name        string                      `json:"name"`
	Description string                      `json:"description"`
	Steps       []SkillWorkflowStepTemplate `json:"steps"`
}

type SkillWorkflowStepTemplate struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Keywords    []string `json:"keywords"`
	Tool        string   `json:"tool,omitempty"`
}

type LLMConfig struct {
	BaseURL string `json:"baseURL"`
	APIKey  string `json:"apiKey"`
	Model   string `json:"model"`
}

func LoadConfig(configPath string) (*AgentConfig, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}
	var cfg AgentConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}
	return &cfg, nil
}
