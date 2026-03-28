package telegram

import (
	"context"
	"fmt"
	"log"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const maxMessageRunes = 4096

func splitMessages(s string) []string {
	r := []rune(s)
	if len(r) == 0 {
		return nil
	}
	var out []string
	for len(r) > 0 {
		n := maxMessageRunes
		if len(r) < n {
			n = len(r)
		}
		out = append(out, string(r[:n]))
		r = r[n:]
	}
	return out
}

// OnTelegramMessage is invoked for each allowed text message; chatID is the sender chat for replies.
// Return one or more strings to send as separate messages in order (e.g. tool output then trajectory note).
type OnTelegramMessage func(ctx context.Context, chatID int64, text string) ([]string, error)

func (c Ingress) RunPoll(ctx context.Context, onMessage OnTelegramMessage) error {
	bot, err := tgbotapi.NewBotAPI(c.Token)
	if err != nil {
		return fmt.Errorf("telegram ingress: bot: %w", err)
	}
	log.Printf("telegram ingress: long poll @%s (shared conversation_id in config uses same TaskStore key as HTTP)", bot.Self.UserName)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := bot.GetUpdatesChan(u)

	go func() {
		<-ctx.Done()
		bot.StopReceivingUpdates()
	}()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case upd, ok := <-updates:
			if !ok {
				return nil
			}
			if upd.Message == nil {
				continue
			}
			msg := upd.Message
			if msg.Text == "" {
				continue
			}
			chatID := msg.Chat.ID
			if !c.AllowChat(chatID) {
				log.Printf("telegram ingress: drop chat_id=%d (not allowed)", chatID)
				continue
			}
			replies, runErr := onMessage(ctx, chatID, msg.Text)
			if runErr != nil {
				if _, err := bot.Send(tgbotapi.NewMessage(chatID, "error: "+runErr.Error())); err != nil {
					log.Printf("telegram ingress: send error message chat_id=%d: %v", chatID, err)
				}
				continue
			}
			for _, reply := range replies {
				for _, part := range splitMessages(reply) {
					if _, err := bot.Send(tgbotapi.NewMessage(chatID, part)); err != nil {
						log.Printf("telegram ingress: send chat_id=%d: %v", chatID, err)
					}
				}
			}
		}
	}
}
