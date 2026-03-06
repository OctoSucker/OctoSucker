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

	return skillConfigs
}
