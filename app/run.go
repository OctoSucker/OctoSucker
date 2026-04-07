package app

import (
	"context"
	"fmt"
	"log"

	"github.com/OctoSucker/octosucker/pkg/repl"
	"github.com/OctoSucker/octosucker/engine/types"
	"github.com/google/uuid"
)

// RunInput handles one Telegram turn. Each turn gets a new task id (random UUID).
func (a *App) RunInputFromTelegram(ctx context.Context, _ int64, text string) ([]string, error) {
	return a.runTurn(ctx, text)
}

// RunInputLocal handles one local CMD / stdin turn. Each turn gets a new task id (random UUID).
func (a *App) RunInputFromLocal(ctx context.Context, text string) ([]string, error) {
	return a.runTurn(ctx, text)
}

func (a *App) runTurn(ctx context.Context, text string) ([]string, error) {
	if !a.turnMu.TryLock() {
		return []string{repl.MsgAgentBusy}, nil
	}
	defer a.turnMu.Unlock()

	taskID := uuid.New().String()
	ev := types.Event{Type: types.EvUserInput, Payload: types.PayloadUserInput{
		TaskID: taskID,
		Text:   text,
	}}
	if err := a.Dispatcher.Run(ctx, ev); err != nil {
		log.Printf("app.runTurn: dispatcher error task=%s err=%v", taskID, err)
		return nil, err
	}
	task, ok := a.Dispatcher.Planner.Tasks.Get(taskID)
	if !ok || task == nil {
		return nil, fmt.Errorf("task missing")
	}
	return task.UserFacingTurnMessages()
}
