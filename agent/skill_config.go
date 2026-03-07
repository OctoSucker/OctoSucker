package agent

import (
	"github.com/OctoSucker/octosucker/config"
)

// buildSkillConfigsFromAgentConfig 从配置文件构建 Skill 配置映射
func buildSkillConfigsFromAgentConfig(cfg *config.AgentConfig) map[string]map[string]interface{} {
	skillConfigs := make(map[string]map[string]interface{})

	// Telegram Skill 配置
	if cfg.Telegram != nil {
		telegramConfig := make(map[string]interface{})
		telegramConfig["bot_token"] = cfg.Telegram.BotToken
		if cfg.Telegram.DMPolicy != "" {
			telegramConfig["dm_policy"] = cfg.Telegram.DMPolicy
		}
		if cfg.Telegram.GroupPolicy != "" {
			telegramConfig["group_policy"] = cfg.Telegram.GroupPolicy
		}
		telegramConfig["require_mention"] = cfg.Telegram.RequireMention
		if len(cfg.Telegram.AllowedChatIDs) > 0 {
			telegramConfig["allowed_chat_ids"] = cfg.Telegram.AllowedChatIDs
		}
		if len(cfg.Telegram.AllowedUserIDs) > 0 {
			telegramConfig["allowed_user_ids"] = cfg.Telegram.AllowedUserIDs
		}
		if cfg.Telegram.MaxMessageStore > 0 {
			telegramConfig["max_message_store_size"] = cfg.Telegram.MaxMessageStore
		}
		skillConfigs["github.com/OctoSucker/skill-telegram"] = telegramConfig
	}

	// 文件系统 Skill 配置
	if cfg.Fs != nil && len(cfg.Fs.WorkspaceDirs) > 0 {
		fsConfig := make(map[string]interface{})
		dirs := make([]interface{}, len(cfg.Fs.WorkspaceDirs))
		for i, d := range cfg.Fs.WorkspaceDirs {
			dirs[i] = d
		}
		fsConfig["workspace_dirs"] = dirs
		skillConfigs["github.com/OctoSucker/skill-fs"] = fsConfig
	}

	// Web Skill 配置
	if cfg.Web != nil && cfg.Web.FetchMaxChars > 0 {
		skillConfigs["github.com/OctoSucker/skill-web"] = map[string]interface{}{
			"fetch_max_chars": cfg.Web.FetchMaxChars,
		}
	}

	return skillConfigs
}
