package app

import (
	"context"
	"fmt"
	"log"
	"strconv"

	"github.com/OctoSucker/agent/pkg/ports"
)

const (
	// TaskSeedCMD is the stable task key for local stdin REPL (distinct from telegram:<chat_id>).
	TaskSeedCMD = "cmd"
	// MsgAgentBusy is returned when another ingress holds the single-flight turn lock.
	MsgAgentBusy = "当前 agent 正忙，请稍后再试。"
)

// RunInput handles one Telegram turn (task id seed: telegram:<chatID>).
func (a *App) RunInput(ctx context.Context, telegramChatID int64, text string) ([]string, error) {
	seed := "telegram:" + strconv.FormatInt(telegramChatID, 10)
	return a.runTurn(ctx, seed, telegramChatID, text)
}

// RunInputLocal handles one local CMD / stdin turn (task id seed: TaskSeedCMD).
func (a *App) RunInputLocal(ctx context.Context, text string) ([]string, error) {
	return a.runTurn(ctx, TaskSeedCMD, 0, text)
}

func (a *App) runTurn(ctx context.Context, taskSeed string, telegramChatID int64, text string) ([]string, error) {
	if !a.turnMu.TryLock() {
		return []string{MsgAgentBusy}, nil
	}
	defer a.turnMu.Unlock()

	taskID := ports.NewTaskIDFromSeed(taskSeed)
	event := ports.Event{Type: ports.EvUserInput, Payload: ports.PayloadUserInput{
		TaskID: taskID,
		Text:   text,
	}}
	if err := a.Dispatcher.Run(ctx, event); err != nil {
		log.Printf("app.runTurn: dispatcher error task=%s err=%v", taskID, err)
		return nil, err
	}
	task, ok := a.Dispatcher.Planner.Tasks.Get(taskID)
	if !ok || task == nil {
		return nil, fmt.Errorf("task missing")
	}
	return task.UserFacingTurnMessages()
}
