package telegram

import (
	"fmt"
	"os"
	"strconv"
	"strings"
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
		return c, fmt.Errorf("telegram bot token is required")
	}
	if defaultChat == 0 {
		return c, fmt.Errorf("telegram default chat id is required")
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

func LoadFromEnv() (Config, error) {
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	var defaultChat int64
	if s := os.Getenv("TELEGRAM_DEFAULT_CHAT_ID"); s != "" {
		id, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return Config{}, fmt.Errorf("TELEGRAM_DEFAULT_CHAT_ID: %w", err)
		}
		defaultChat = id
	}
	var allowed []int64
	raw := os.Getenv("TELEGRAM_ALLOWED_CHAT_IDS")
	if raw != "" {
		for _, part := range strings.Split(raw, ",") {
			if part == "" {
				continue
			}
			id, err := strconv.ParseInt(part, 10, 64)
			if err != nil {
				return Config{}, fmt.Errorf("TELEGRAM_ALLOWED_CHAT_IDS: invalid id %q", part)
			}
			allowed = append(allowed, id)
		}
	}
	return NewConfig(token, defaultChat, allowed)
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
