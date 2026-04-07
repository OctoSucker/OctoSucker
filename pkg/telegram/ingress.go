package telegram

import (
	"fmt"
)

type Ingress struct {
	Token       string
	DefaultChat int64
	Allowed     map[int64]struct{}
}

func NewIngress(token string, defaultChat int64, allowedChatIDs []int64) (*Ingress, error) {
	c := &Ingress{}
	c.Token = token
	if c.Token == "" {
		return nil, fmt.Errorf("telegram bot token is required")
	}
	if defaultChat == 0 {
		return nil, fmt.Errorf("telegram default chat id is required")
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

func (c Ingress) AllowChat(id int64) bool {
	if _, ok := c.Allowed[id]; ok {
		return true
	}
	return id == c.DefaultChat
}
