package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
)

type Workspace struct {
	OpenAI      OpenAI   `json:"openai"`
	MCPEndpoint []string `json:"mcp_endpoint"`
	HTTP        HTTP     `json:"http"`
	Telegram    Telegram `json:"telegram"`
	// ConversationID: when non-empty, all channels use this TaskStore key (one rolling thread).
	// HTTP POST /run may omit task_id; Telegram still sends replies to the incoming chat_id.
	ConversationID string `json:"conversation_id,omitempty"`
	// GraphPathMode: "greedy" (default) = Frontier local ranking; "global" = global edge-cost selection among feasible next caps.
	GraphPathMode string `json:"graph_path_mode,omitempty"`
	// SkillLearnMinPlanSteps: min plan steps before skill extract counter applies (0 = default 3; -1 = no minimum).
	SkillLearnMinPlanSteps int `json:"skill_learn_min_plan_steps,omitempty"`
	// SkillLearnMinSuccessCount: qualifying successes per capability path before MergeOrAdd (0 = default 2).
	SkillLearnMinSuccessCount int `json:"skill_learn_min_success_count,omitempty"`
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
	cfg.ConversationID = strings.TrimSpace(cfg.ConversationID)
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
	if len(f.MCPEndpoint) == 0 {
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
