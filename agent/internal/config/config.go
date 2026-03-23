package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
)

type Workspace struct {
	OpenAI      OpenAI   `json:"openai"`
	MCPEndpoint string   `json:"mcp_endpoint"`
	HTTP        HTTP     `json:"http"`
	Telegram    Telegram `json:"telegram"`
}

type OpenAI struct {
	APIKey         string `json:"api_key"`
	BaseURL        string `json:"base_url"`
	Model          string `json:"model"`
	EmbeddingModel string `json:"embedding_model"`
}

type HTTP struct {
	Listen string `json:"listen"`
}

type Telegram struct {
	BotToken       string  `json:"bot_token"`
	DefaultChatID  int64   `json:"default_chat_id"`
	AllowedChatIDs []int64 `json:"allowed_chat_ids,omitempty"`
}

func LoadWorkspace(workspaceRoot string) (*Workspace, error) {
	p := ConfigFile(workspaceRoot)
	raw, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("missing %s: copy workspace/config.example.json (repo root) to %s and set openai + mcp.endpoint", p, p)
		}
		return nil, fmt.Errorf("read %s: %w", p, err)
	}
	var cfg Workspace
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", p, err)
	}
	if err := cfg.normalizeAndValidate(p); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (f *Workspace) normalizeAndValidate(path string) error {
	if f.HTTP.Listen == "" {
		f.HTTP.Listen = ":8080"
	}
	if f.OpenAI.APIKey == "" {
		return fmt.Errorf("%s: openai.api_key is required", path)
	}
	if f.OpenAI.BaseURL == "" {
		return fmt.Errorf("%s: openai.base_url is required", path)
	}
	if f.OpenAI.Model == "" {
		return fmt.Errorf("%s: openai.model is required", path)
	}
	if f.OpenAI.EmbeddingModel == "" {
		return fmt.Errorf("%s: openai.embedding_model is required", path)
	}
	if f.MCPEndpoint == "" {
		return fmt.Errorf("%s: mcp_endpoint is required", path)
	}
	if f.Telegram.BotToken == "" {
		return fmt.Errorf("%s: telegram.bot_token is required", path)
	}
	if f.Telegram.DefaultChatID == 0 {
		return fmt.Errorf("%s: telegram.default_chat_id is required", path)
	}
	return nil
}
