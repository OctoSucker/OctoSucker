package telegrambuiltin

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/OctoSucker/agent/internal/config"
	"github.com/OctoSucker/agent/pkg/ports"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Runner struct {
	cfg Config
	api *tgbotapi.BotAPI
}

func NewRunner(t config.Telegram) (*Runner, error) {
	cfg, err := ConfigFromWorkspace(t)
	if err != nil {
		return nil, err
	}
	api, err := tgbotapi.NewBotAPI(cfg.Token)
	if err != nil {
		return nil, fmt.Errorf("telegram builtin: bot api: %w", err)
	}
	return &Runner{cfg: cfg, api: api}, nil
}

// Name is the ToolRegistry.Backends map key for this provider (not a user-facing tool id).
func (r *Runner) Name() string { return "telegram" }

func (r *Runner) HasTool(tool string) bool {
	switch strings.TrimSpace(tool) {
	case "send_telegram_message", "get_telegram_chat", "get_telegram_bot_info", "get_telegram_allowed_chat_ids":
		return true
	default:
		return false
	}
}

func (r *Runner) Tool(tool string) (*mcp.Tool, error) {
	name := strings.TrimSpace(tool)
	switch name {
	case "send_telegram_message":
		return &mcp.Tool{
			Name:        name,
			Description: "Send a Telegram text message. Uses default_chat_id from workspace when chat_id is omitted.",
			InputSchema: schemaSendMessage(),
		}, nil
	case "get_telegram_chat":
		return &mcp.Tool{
			Name:        name,
			Description: "Fetch Telegram chat metadata by chat_id.",
			InputSchema: schemaGetChat(),
		}, nil
	case "get_telegram_bot_info":
		return &mcp.Tool{
			Name:        name,
			Description: "Return the current bot user (GetMe).",
			InputSchema: schemaEmpty(),
		}, nil
	case "get_telegram_allowed_chat_ids":
		return &mcp.Tool{
			Name:        name,
			Description: "Lists allowed chat IDs from allowed_chat_ids and default chat. Use these values for send_telegram_message.chat_id when enforcing allowlists.",
			InputSchema: schemaEmpty(),
		}, nil
	default:
		return nil, fmt.Errorf("telegram builtin: unknown tool %q", tool)
	}
}

func (r *Runner) ToolList(ctx context.Context) ([]*mcp.Tool, error) {
	tools := []string{
		"send_telegram_message",
		"get_telegram_chat",
		"get_telegram_bot_info",
		"get_telegram_allowed_chat_ids",
	}
	out := make([]*mcp.Tool, 0, len(tools))
	for _, t := range tools {
		tool, err := r.Tool(t)
		if err != nil {
			return nil, err
		}
		out = append(out, tool)
	}
	return out, nil
}

func (r *Runner) Invoke(ctx context.Context, localTool string, arguments map[string]any) (ports.ToolResult, error) {
	if !r.HasTool(localTool) {
		return ports.ToolResult{Err: fmt.Errorf("telegram builtin: unknown tool %q", localTool)}, fmt.Errorf("telegram builtin: unknown tool %q", localTool)
	}
	var out string
	switch localTool {
	case "send_telegram_message":
		var args sendArgs
		if err := decodeArgs(arguments, &args); err != nil {
			return ports.ToolResult{Err: fmt.Errorf("telegram builtin: arguments: %w", err)}, fmt.Errorf("telegram builtin: arguments: %w", err)
		}
		out = r.toolSendTelegramMessage(ctx, args)
	case "get_telegram_chat":
		var args chatArgs
		if err := decodeArgs(arguments, &args); err != nil {
			return ports.ToolResult{Err: fmt.Errorf("telegram builtin: arguments: %w", err)}, fmt.Errorf("telegram builtin: arguments: %w", err)
		}
		out = r.toolGetTelegramChat(ctx, args)
	case "get_telegram_bot_info":
		out = r.toolGetTelegramBotInfo(ctx)
	case "get_telegram_allowed_chat_ids":
		out = r.toolGetTelegramAllowedChatIDs(ctx)
	default:
		return ports.ToolResult{Err: fmt.Errorf("telegram builtin: unknown tool %q", localTool)}, fmt.Errorf("telegram builtin: unknown tool %q", localTool)
	}
	return ports.ToolResult{Output: out}, nil
}

func decodeArgs(m map[string]any, dst any) error {
	if m == nil {
		m = map[string]any{}
	}
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, dst)
}

func jsonText(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(b)
}

func schemaEmpty() map[string]any {
	return map[string]any{
		"type":                 "object",
		"properties":           map[string]any{},
		"additionalProperties": false,
	}
}

func schemaGetChat() map[string]any {
	return map[string]any{
		"type":     "object",
		"required": []string{"chat_id"},
		"properties": map[string]any{
			"chat_id": map[string]any{"type": "integer", "description": "telegram chat id"},
		},
		"additionalProperties": false,
	}
}

func schemaSendMessage() map[string]any {
	return map[string]any{
		"type":     "object",
		"required": []string{"text"},
		"properties": map[string]any{
			"text": map[string]any{"type": "string", "description": "message text"},
			"chat_id": map[string]any{
				"type":        "integer",
				"description": "target chat; omit to use workspace default_chat_id",
			},
			"parse_mode": map[string]any{
				"type":        "string",
				"description": "Markdown, MarkdownV2, HTML, or empty",
			},
			"disable_web_page_preview": map[string]any{"type": "boolean"},
			"reply_to_message_id":      map[string]any{"type": "integer"},
		},
		"additionalProperties": false,
	}
}

type sendArgs struct {
	Text                  string `json:"text"`
	ChatID                *int64 `json:"chat_id,omitempty"`
	ParseMode             string `json:"parse_mode,omitempty"`
	DisableWebPagePreview bool   `json:"disable_web_page_preview,omitempty"`
	ReplyToMessageID      int    `json:"reply_to_message_id,omitempty"`
}

type chatArgs struct {
	ChatID int64 `json:"chat_id"`
}

func (r *Runner) toolSendTelegramMessage(ctx context.Context, args sendArgs) string {
	_ = ctx
	if args.Text == "" {
		return jsonText(map[string]any{"ok": false, "error": "text is required"})
	}
	chatID, err := r.cfg.ResolveChat(args.ChatID)
	if err != nil {
		return jsonText(map[string]any{"ok": false, "error": err.Error()})
	}
	msg := tgbotapi.NewMessage(chatID, args.Text)
	if args.ParseMode != "" {
		msg.ParseMode = args.ParseMode
	}
	msg.DisableWebPagePreview = args.DisableWebPagePreview
	if args.ReplyToMessageID > 0 {
		msg.ReplyToMessageID = args.ReplyToMessageID
	}
	sent, err := r.api.Send(msg)
	if err != nil {
		return jsonText(map[string]any{"ok": false, "error": err.Error()})
	}
	return jsonText(map[string]any{"ok": true, "chat_id": chatID, "message_id": sent.MessageID})
}

func (r *Runner) toolGetTelegramChat(ctx context.Context, args chatArgs) string {
	_ = ctx
	if !r.cfg.AllowChat(args.ChatID) {
		return jsonText(map[string]any{"ok": false, "error": "chat_id not allowed"})
	}
	chat, err := r.api.GetChat(tgbotapi.ChatInfoConfig{ChatConfig: tgbotapi.ChatConfig{ChatID: args.ChatID}})
	if err != nil {
		return jsonText(map[string]any{"ok": false, "error": err.Error()})
	}
	return jsonText(map[string]any{
		"ok": true,
		"chat": map[string]any{
			"id": chat.ID, "type": chat.Type, "title": chat.Title, "username": chat.UserName,
		},
	})
}

func (r *Runner) toolGetTelegramBotInfo(ctx context.Context) string {
	_ = ctx
	me, err := r.api.GetMe()
	if err != nil {
		return jsonText(map[string]any{"ok": false, "error": err.Error()})
	}
	return jsonText(map[string]any{
		"ok": true,
		"bot": map[string]any{
			"id": me.ID, "username": me.UserName, "first_name": me.FirstName, "last_name": me.LastName,
		},
	})
}

func (r *Runner) toolGetTelegramAllowedChatIDs(ctx context.Context) string {
	_ = ctx
	ids := make([]int64, 0, len(r.cfg.Allowed))
	for id := range r.cfg.Allowed {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	out := map[string]any{
		"ok": true, "allowed_chat_ids": ids, "default_chat_id": r.cfg.DefaultChat,
	}
	if len(r.cfg.Allowed) == 0 {
		out["note"] = "only default_chat_id is allowed unless listed in allowed_chat_ids"
	}
	return jsonText(out)
}
