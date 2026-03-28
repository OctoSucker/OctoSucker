package app

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/OctoSucker/agent/pkg/ports"
)

// RunInput runs one user turn and returns messages to send in order.
// Reply and TrajectorySummary are produced by TrajectoryCritic (Reply excludes a successful final
// send_telegram_message so the user is not spammed with duplicate content).
func (a *App) RunInput(ctx context.Context, telegramChatID int64, text string) ([]string, error) {
	taskID := ports.NewTaskIDFromSeed("telegram:" + strconv.FormatInt(telegramChatID, 10))
	event := ports.Event{Type: ports.EvUserInput, Payload: ports.PayloadUserInput{
		TaskID:         taskID,
		Text:           text,
		TelegramChatID: telegramChatID,
	}}
	if err := a.Dispatcher.Run(ctx, event); err != nil {
		log.Printf("app.RunInput: dispatcher error task=%s err=%v", event.Payload.(ports.PayloadUserInput).TaskID, err)
		return nil, err
	}
	taskState, ok := a.Dispatcher.Planner.Tasks.Get(taskID)
	if !ok || taskState == nil {
		return nil, fmt.Errorf("task missing")
	}
	reply := strings.TrimSpace(taskState.Reply)
	traj := strings.TrimSpace(taskState.TrajectorySummary)
	if reply == "" && traj == "" {
		return nil, fmt.Errorf("task has empty reply")
	}
	var msgs []string
	if reply != "" {
		msgs = append(msgs, taskState.Reply)
	}
	if traj != "" {
		msgs = append(msgs, taskState.TrajectorySummary)
	}
	return msgs, nil
}
