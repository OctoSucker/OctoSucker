package telegram

import (
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const ImplementationName = "octoplus-mcp-telegram"

func NewBotAPI(cfg Config) (*tgbotapi.BotAPI, error) {
	return tgbotapi.NewBotAPI(cfg.Token)
}

func RegisterTools(srv *mcp.Server, api *tgbotapi.BotAPI, cfg Config) {
	registerTelegramTools(srv, api, cfg)
}

func NewMCPServer(cfg Config, api *tgbotapi.BotAPI) *mcp.Server {
	srv := mcp.NewServer(&mcp.Implementation{Name: ImplementationName, Version: "0.1"}, nil)
	RegisterTools(srv, api, cfg)
	return srv
}
