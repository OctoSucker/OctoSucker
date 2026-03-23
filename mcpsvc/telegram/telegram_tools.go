package telegram

import (
	"context"

	"github.com/OctoSucker/mcpsvc/internal/mcpx"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerTelegramTools(srv *mcp.Server, api *tgbotapi.BotAPI, cfg Config) {
	type sendArgs struct {
		Text                  string `json:"text" jsonschema:"message text"`
		ChatID                *int64 `json:"chat_id,omitempty" jsonschema:"target chat; omit to use TELEGRAM_DEFAULT_CHAT_ID"`
		ParseMode             string `json:"parse_mode,omitempty" jsonschema:"Markdown, MarkdownV2, HTML, or empty"`
		DisableWebPagePreview bool   `json:"disable_web_page_preview,omitempty"`
		ReplyToMessageID      int    `json:"reply_to_message_id,omitempty"`
	}
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "send_telegram_message",
		Description: "Send a Telegram text message. Uses TELEGRAM_DEFAULT_CHAT_ID when chat_id is omitted.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args sendArgs) (*mcp.CallToolResult, any, error) {
		if args.Text == "" {
			return mcpx.TextResult(`{"ok":false,"error":"text is required"}`), nil, nil
		}
		chatID, err := cfg.ResolveChat(args.ChatID)
		if err != nil {
			return mcpx.TextResult(mcpx.JSONText(map[string]any{"ok": false, "error": err.Error()})), nil, nil
		}
		msg := tgbotapi.NewMessage(chatID, args.Text)
		if args.ParseMode != "" {
			msg.ParseMode = args.ParseMode
		}
		msg.DisableWebPagePreview = args.DisableWebPagePreview
		if args.ReplyToMessageID > 0 {
			msg.ReplyToMessageID = args.ReplyToMessageID
		}
		sent, err := api.Send(msg)
		if err != nil {
			return mcpx.TextResult(mcpx.JSONText(map[string]any{"ok": false, "error": err.Error()})), nil, nil
		}
		return mcpx.TextResult(mcpx.JSONText(map[string]any{"ok": true, "chat_id": chatID, "message_id": sent.MessageID})), nil, nil
	})

	type chatArgs struct {
		ChatID int64 `json:"chat_id" jsonschema:"telegram chat id"`
	}
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "get_telegram_chat",
		Description: "Fetch Telegram chat metadata by chat_id.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args chatArgs) (*mcp.CallToolResult, any, error) {
		if !cfg.AllowChat(args.ChatID) {
			return mcpx.TextResult(mcpx.JSONText(map[string]any{"ok": false, "error": "chat_id not allowed"})), nil, nil
		}
		chat, err := api.GetChat(tgbotapi.ChatInfoConfig{ChatConfig: tgbotapi.ChatConfig{ChatID: args.ChatID}})
		if err != nil {
			return mcpx.TextResult(mcpx.JSONText(map[string]any{"ok": false, "error": err.Error()})), nil, nil
		}
		return mcpx.TextResult(mcpx.JSONText(map[string]any{
			"ok": true,
			"chat": map[string]any{
				"id": chat.ID, "type": chat.Type, "title": chat.Title, "username": chat.UserName,
			},
		})), nil, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "get_telegram_bot_info",
		Description: "Return the current bot user (GetMe).",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
		me, err := api.GetMe()
		if err != nil {
			return mcpx.TextResult(mcpx.JSONText(map[string]any{"ok": false, "error": err.Error()})), nil, nil
		}
		return mcpx.TextResult(mcpx.JSONText(map[string]any{
			"ok": true,
			"bot": map[string]any{
				"id": me.ID, "username": me.UserName, "first_name": me.FirstName, "last_name": me.LastName,
			},
		})), nil, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "get_telegram_allowed_chat_ids",
		Description: "Lists allowed chat IDs from TELEGRAM_ALLOWED_CHAT_IDS and TELEGRAM_DEFAULT_CHAT_ID. Use these values for send_telegram_message.chat_id when enforcing allowlists.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
		ids := make([]int64, 0, len(cfg.Allowed))
		for id := range cfg.Allowed {
			ids = append(ids, id)
		}
		out := map[string]any{
			"ok": true, "allowed_chat_ids": ids, "default_chat_id": cfg.DefaultChat,
		}
		if len(cfg.Allowed) == 0 {
			out["note"] = "only default_chat_id is allowed unless listed in allowed_chat_ids"
		}
		return mcpx.TextResult(mcpx.JSONText(out)), nil, nil
	})
}
