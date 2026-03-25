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
	sess, ok := s.Get(taskID)
	if !ok || sess == nil {
		return "", false
	}
	return sess.Reply, true
}

func (a *App) RerunTaskPlan(ctx context.Context, pathTaskID string) (string, error) {
	tid, _, err := a.effectiveTaskID(pathTaskID, 0)
	if err != nil {
		return "", err
	}
	if a == nil || a.Dispatcher == nil || a.Dispatcher.Planner == nil || a.Dispatcher.Planner.Tasks == nil {
		return "", fmt.Errorf("app: nil dispatcher or task store")
	}
	sessStore := a.Dispatcher.Planner.Tasks
	sess, ok := sessStore.Get(tid)
	if !ok || sess == nil || sess.Plan == nil {
		return "", ErrRerunNoPlan
	}
	for i := range sess.Plan.Steps {
		sess.Plan.Steps[i].Status = "pending"
	}
	sess.Trace = nil
	sess.ToolFailCount = nil
	sess.CapabilityFailCount = nil
	sess.CapChainStepID = ""
	sess.CapChainTools = nil
	sess.CapChainNext = 0
	sess.StepID = ""
	sess.PendingTool = ""
	sess.LastCapability = ""
	sess.LastOutcome = 0
	sess.Reply = ""
	sess.TrajectoryScore = 0
	sess.TrajectorySummary = ""
	sess.ReplanAllowed = true
	sess.ReplanCount = 0
	if err := sessStore.Put(sess); err != nil {
		return "", err
	}
	event := ports.Event{Type: ports.EvPlanProgressed, Payload: ports.PayloadPlanProgressed{TaskID: tid}}
	if err := a.Dispatcher.Run(ctx, event); err != nil {
		log.Printf("app.RerunTaskPlan: dispatcher error task=%s err=%v", tid, err)
		return "", err
	}
	reply, ok := replyFromStore(sessStore, tid)
	if !ok {
		return "", fmt.Errorf("task missing")
	}
	return reply, nil
}
