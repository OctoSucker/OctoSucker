package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/OctoSucker/agent/internal/runtime/store/task"
	"github.com/OctoSucker/agent/pkg/ports"
	rtutils "github.com/OctoSucker/agent/utils"
)

var ErrRerunNoPlan = errors.New("task or plan missing")

// effectiveTaskID maps ingress to canonical Task UUID and optional Telegram tool hint.
func (a *App) effectiveTaskID(clientTaskID string, telegramChatID int64) (tid string, telegramHint int64, err error) {
	unified := a != nil && strings.TrimSpace(a.ConversationID) != ""
	if unified {
		tid = ports.NewTaskIDFromSeed("conversation:" + strings.TrimSpace(a.ConversationID))
		if telegramChatID != 0 {
			telegramHint = telegramChatID
		}
		return tid, telegramHint, nil
	}
	if telegramChatID != 0 {
		return ports.NewTaskIDFromSeed("telegram:" + strconv.FormatInt(telegramChatID, 10)), telegramChatID, nil
	}
	if clientTaskID == "" {
		return "", 0, fmt.Errorf("task id required")
	}
	norm := rtutils.HTTPNormalizeChatTaskID(clientTaskID)
	return ports.NewTaskIDFromSeed("http:" + norm), 0, nil
}

func (a *App) RunInput(ctx context.Context, clientTaskID, text string, telegramChatID int64) (string, error) {
	tid, tgHint, err := a.effectiveTaskID(clientTaskID, telegramChatID)
	if err != nil {
		return "", err
	}
	event := ports.Event{Type: ports.EvUserInput, Payload: ports.PayloadUserInput{
		TaskID: tid, Text: text, TelegramChatID: tgHint,
	}}
	if err := a.Dispatcher.Run(ctx, event); err != nil {
		log.Printf("app.RunInput: dispatcher error task=%s err=%v", tid, err)
		return "", err
	}
	reply, ok := replyFromStore(a.Dispatcher.Planner.Tasks, tid)
	if !ok {
		return "", fmt.Errorf("task missing")
	}
	return reply, nil
}

func replyFromStore(s *task.TaskStore, taskID string) (string, bool) {
	if s == nil || taskID == "" {
		return "", false
	}
	taskState, ok := s.Get(taskID)
	if !ok || taskState == nil {
		return "", false
	}
	return taskState.Reply, true
}

func (a *App) RerunTaskPlan(ctx context.Context, pathTaskID string) (string, error) {
	tid, _, err := a.effectiveTaskID(pathTaskID, 0)
	if err != nil {
		return "", err
	}
	if a == nil || a.Dispatcher == nil || a.Dispatcher.Planner == nil || a.Dispatcher.Planner.Tasks == nil {
		return "", fmt.Errorf("app: nil dispatcher or task store")
	}
	taskStore := a.Dispatcher.Planner.Tasks
	taskState, ok := taskStore.Get(tid)
	if !ok || taskState == nil || taskState.Plan == nil {
		return "", ErrRerunNoPlan
	}
	for i := range taskState.Plan.Steps {
		taskState.Plan.Steps[i].Status = "pending"
	}
	taskState.Trace = nil
	taskState.ToolFailCount = nil
	taskState.CapabilityFailCount = nil
	taskState.CapChainStepID = ""
	taskState.CapChainTools = nil
	taskState.CapChainNext = 0
	taskState.StepID = ""
	taskState.PendingTool = ""
	taskState.LastCapability = ""
	taskState.LastOutcome = 0
	taskState.Reply = ""
	taskState.TrajectoryScore = 0
	taskState.TrajectorySummary = ""
	taskState.ReplanAllowed = true
	taskState.ReplanCount = 0
	if err := taskStore.Put(taskState); err != nil {
		return "", err
	}
	event := ports.Event{Type: ports.EvPlanProgressed, Payload: ports.PayloadPlanProgressed{TaskID: tid}}
	if err := a.Dispatcher.Run(ctx, event); err != nil {
		log.Printf("app.RerunTaskPlan: dispatcher error task=%s err=%v", tid, err)
		return "", err
	}
	reply, ok := replyFromStore(taskStore, tid)
	if !ok {
		return "", fmt.Errorf("task missing")
	}
	return reply, nil
}
