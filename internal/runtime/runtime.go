// Package runtime is the workspace-scoped agent process core: SQLite, dispatcher,
// single-flight user turns, and exposure of read-only KG access for the admin HTTP layer.
// Ingress adapters live under internal/ingress, wired by internal/gateway.
package runtime

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/OctoSucker/octosucker/config"
	types "github.com/OctoSucker/octosucker/internal/runtime/model"
	"github.com/OctoSucker/octosucker/internal/storage"
	"github.com/google/uuid"
)

// MsgBusy is returned when a second turn starts while another holds the single-flight lock.
const MsgBusy = "当前 agent 正忙，请稍后再试。"

// Runtime is one loaded workspace: dispatcher plus persistent store.
type Runtime struct {
	Dispatcher *Dispatcher
	data       *storage.DB
	turnMu     sync.Mutex
}

// NewRuntime loads workspace SQLite and builds the dispatcher from cfg.
func NewRuntime(ctx context.Context, workspaceRoot string, cfg *config.Workspace) (*Runtime, error) {
	if cfg == nil {
		return nil, fmt.Errorf("runtime: workspace config required")
	}
	data, err := storage.Open(workspaceRoot)
	if err != nil {
		return nil, fmt.Errorf("runtime: sqlite: %w", err)
	}

	d, err := NewDispatcher(ctx, DispatcherDeps{
		WorkspaceRoot: workspaceRoot,
		MCPEndpoints:  cfg.MCPEndpoint,
		OpenAI:        cfg.OpenAI,
		Exec:          cfg.Exec,
		Telegram:      cfg.Telegram,
		SkillsDir:     cfg.SkillsDir,
		Data:          data,
	})
	if err != nil {
		if cerr := data.Close(); cerr != nil {
			err = errors.Join(err, fmt.Errorf("close data db: %w", cerr))
		}
		return nil, fmt.Errorf("runtime: dispatcher: %w", err)
	}

	return &Runtime{
		Dispatcher: d,
		data:       data,
	}, nil
}

// WorkspaceDB returns the workspace store for admin KG APIs, or nil after Close.
func (r *Runtime) WorkspaceDB() storage.KnowledgeGraphReader {
	return r.data
}

// RunTurn handles one user message from any ingress (new task id per call).
func (r *Runtime) RunTurn(ctx context.Context, text string) ([]string, error) {
	if !r.turnMu.TryLock() {
		return []string{MsgBusy}, nil
	}
	defer r.turnMu.Unlock()

	taskID := uuid.New().String()
	ev := types.Event{Type: types.EvTurnRequested, Payload: types.TurnRequest{
		TaskID: taskID,
		Text:   text,
	}}
	if err := r.Dispatcher.Run(ctx, ev); err != nil {
		log.Printf("runtime.RunTurn: dispatcher error task=%s err=%v", taskID, err)
		return []string{friendlyRunError(err)}, nil
	}
	task, ok := r.Dispatcher.Planner.Tasks.Get(taskID)
	if !ok || task == nil {
		return nil, fmt.Errorf("task missing")
	}
	return task.UserFacingTurnMessages()
}

func friendlyRunError(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	lower := strings.ToLower(msg)
	switch {
	case errors.Is(err, context.DeadlineExceeded), strings.Contains(lower, "timeout"), strings.Contains(lower, "deadline exceeded"):
		return "这次模型或工具调用超时了。你可以稍后重试，或把请求拆得更短一些。"
	case strings.Contains(lower, "tls handshake timeout"), strings.Contains(lower, "connection refused"), strings.Contains(lower, "no such host"):
		return "网络连接暂时不可用，模型或外部工具没有响应。请稍后重试。"
	case strings.Contains(lower, "validate tool arguments"), strings.Contains(lower, "tool arguments json"), strings.Contains(lower, "unmarshal completion json"):
		return "这次我没有生成有效的工具参数，已停止本轮以避免继续误操作。请换一种更明确的说法再试。"
	case strings.Contains(lower, "unknown tool provider"), strings.Contains(lower, "unknown tool"):
		return "我选择了当前不可用的工具。你可以先让我“列出当前有哪些工具 provider”，再指定要用的工具。"
	case strings.Contains(lower, "goal not met after"):
		return "我尝试了多次仍没有找到可行路径，已停止本轮。请补充更多上下文，或明确希望使用哪个工具。"
	default:
		return "本轮执行失败，详细原因已写入 agent 日志。你可以调整请求后重试。"
	}
}

// Close closes the workspace database and drops dispatcher references.
func (r *Runtime) Close() error {
	var err error
	if r.data != nil {
		if r.Dispatcher != nil && r.Dispatcher.ToolRegistry != nil {
			if cerr := r.Dispatcher.ToolRegistry.Close(); cerr != nil {
				err = errors.Join(err, fmt.Errorf("close tool registry: %w", cerr))
			}
		}
	}
	if r.data != nil {
		if cerr := r.data.Close(); cerr != nil {
			err = errors.Join(err, fmt.Errorf("close data db: %w", cerr))
		}
		r.data = nil
	}
	if r.Dispatcher != nil {
		r.Dispatcher = nil
	}
	return err
}
