package telegrambuiltin

import (
	"fmt"

	"github.com/OctoSucker/agent/internal/config"
)

type Config struct {
	Token       string
	DefaultChat int64
	Allowed     map[int64]struct{}
}

func NewConfig(token string, defaultChat int64, allowedChatIDs []int64) (Config, error) {
	c := Config{}
	c.Token = token
	if c.Token == "" {
		return c, fmt.Errorf("telegram builtin: bot token is required")
	}
	if defaultChat == 0 {
		return c, fmt.Errorf("telegram builtin: default chat id is required")
	}
	c.DefaultChat = defaultChat
	if len(allowedChatIDs) > 0 {
		c.Allowed = make(map[int64]struct{}, len(allowedChatIDs))
		for _, id := range allowedChatIDs {
			c.Allowed[id] = struct{}{}
		}
	}
	return c, nil
}

func ConfigFromWorkspace(t config.Telegram) (Config, error) {
	return NewConfig(t.BotToken, t.DefaultChatID, t.AllowedChatIDs)
}

func (c Config) AllowChat(id int64) bool {
	if _, ok := c.Allowed[id]; ok {
		return true
	}
	return id == c.DefaultChat
}

func (c Config) ResolveChat(explicit *int64) (int64, error) {
	if explicit != nil && *explicit != 0 {
		if !c.AllowChat(*explicit) {
			return 0, fmt.Errorf("chat_id %d is not allowed (use TELEGRAM_ALLOWED_CHAT_IDS or default chat id)", *explicit)
		}
		return *explicit, nil
	}
	return c.DefaultChat, nil
}
