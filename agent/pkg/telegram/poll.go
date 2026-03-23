package telegram

import (
	"context"
	"fmt"
	"log"
	"strconv"

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

func (c Ingress) RunPoll(ctx context.Context, run func(context.Context, string, string) (string, error)) error {
	bot, err := tgbotapi.NewBotAPI(c.Token)
	if err != nil {
		return fmt.Errorf("telegram ingress: bot: %w", err)
	}
	log.Printf("telegram ingress: long poll @%s (session_id = chat_id string, same queue as POST /run)", bot.Self.UserName)

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
			sid := strconv.FormatInt(chatID, 10)
			reply, runErr := run(ctx, sid, msg.Text)
			if runErr != nil {
				_, _ = bot.Send(tgbotapi.NewMessage(chatID, "error: "+runErr.Error()))
				continue
			}
			for _, part := range splitMessages(reply) {
				if _, err := bot.Send(tgbotapi.NewMessage(chatID, part)); err != nil {
					log.Printf("telegram ingress: send chat_id=%d: %v", chatID, err)
				}
			}
		}
	}
}
