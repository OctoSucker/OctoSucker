package app

import (
	"context"
	"fmt"
	"log"
	"strconv"

	"github.com/OctoSucker/agent/pkg/ports"
)

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
	task, ok := a.Dispatcher.Planner.Tasks.Get(taskID)
	if !ok || task == nil {
		return nil, fmt.Errorf("task missing")
	}
	return task.UserFacingTurnMessages()
}
