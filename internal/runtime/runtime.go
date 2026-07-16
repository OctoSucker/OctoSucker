// Package runtime composes the workspace-scoped agent loop and exposes a small
// ingress-facing API.
package runtime

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/OctoSucker/octosucker/config"
	"github.com/OctoSucker/octosucker/internal/interaction"
	"github.com/OctoSucker/octosucker/internal/runtime/agentloop"
	"github.com/OctoSucker/octosucker/internal/runtime/contextmanager"
	"github.com/OctoSucker/octosucker/internal/runtime/conversation"
	"github.com/OctoSucker/octosucker/internal/runtime/evaluation"
	"github.com/OctoSucker/octosucker/internal/runtime/execution"
	"github.com/OctoSucker/octosucker/internal/runtime/learning"
	"github.com/OctoSucker/octosucker/internal/runtime/model"
	"github.com/OctoSucker/octosucker/internal/runtime/planning"
	"github.com/OctoSucker/octosucker/internal/runtime/projectcontext"
	"github.com/OctoSucker/octosucker/internal/runtime/responding"
	"github.com/OctoSucker/octosucker/internal/runtime/toolrouting"
	"github.com/OctoSucker/octosucker/internal/storage"
	"github.com/OctoSucker/octosucker/internal/task"
	"github.com/OctoSucker/octosucker/internal/toolcontract"
	"github.com/OctoSucker/octosucker/internal/tools"
	"github.com/OctoSucker/octosucker/pkg/llmclient"
	"github.com/google/uuid"
)

const MsgBusy = "当前 agent 正忙，请稍后再试。"

type Runtime struct {
	loop          *agentloop.Loop
	tools         *tools.Registry
	conversations *conversation.Store
	interactions  *interaction.Planner
	data          *storage.DB
	tasks         *task.Store
	ctx           context.Context
	turnMu        sync.Mutex
	approvalMu    sync.Mutex
	approvals     map[string]chan bool
}

func NewRuntime(ctx context.Context, workspaceRoot string, cfg *config.Workspace) (*Runtime, error) {
	if cfg == nil {
		return nil, fmt.Errorf("runtime: workspace config required")
	}
	data, err := storage.Open(workspaceRoot)
	if err != nil {
		return nil, fmt.Errorf("runtime: sqlite: %w", err)
	}
	closeData := func(cause error) error {
		if closeErr := data.Close(); closeErr != nil {
			return errors.Join(cause, closeErr)
		}
		return cause
	}

	projectCtx := projectcontext.Load(workspaceRoot)
	plannerLLM := newRoleLLM(cfg.OpenAI, cfg.OpenAI.Models.Planner)
	evaluatorLLM := newRoleLLM(cfg.OpenAI, cfg.OpenAI.Models.Evaluator)
	responderLLM := newRoleLLM(cfg.OpenAI, cfg.OpenAI.Models.Responder)
	contexts := contextmanager.New(contextmanager.Limits{
		PlannerTokens:   cfg.Context.PlannerInputTokens,
		EvaluatorTokens: cfg.Context.EvaluatorInputTokens,
		ResponderTokens: cfg.Context.ResponderInputTokens,
	})
	registry, err := tools.NewRegistry(ctx, tools.RegistryDeps{
		WorkspaceRoot: workspaceRoot,
		MCPEndpoints:  cfg.MCPEndpoint,
		Exec:          cfg.Exec,
		Telegram:      cfg.Telegram,
		OpenCLI:       cfg.OpenCLI,
		SkillsDir:     cfg.SkillsDir,
		EmbedLLM:      plannerLLM,
	})
	if err != nil {
		return nil, closeData(fmt.Errorf("runtime: tool registry: %w", err))
	}
	closeAll := func(cause error) error {
		if closeErr := registry.Close(); closeErr != nil {
			cause = errors.Join(cause, closeErr)
		}
		return closeData(cause)
	}

	routeGraph, err := toolrouting.New(data, registry.AllToolIDs())
	if err != nil {
		return nil, closeAll(fmt.Errorf("runtime: routing advisor: %w", err))
	}
	planner, err := planning.New(registry, plannerLLM, routeGraph, contexts, projectCtx)
	if err != nil {
		return nil, closeAll(err)
	}
	executor, err := execution.New(registry)
	if err != nil {
		return nil, closeAll(err)
	}
	evaluator, err := evaluation.New(evaluatorLLM, contexts, projectCtx)
	if err != nil {
		return nil, closeAll(err)
	}
	responder, err := responding.New(responderLLM, registry, contexts, projectCtx)
	if err != nil {
		return nil, closeAll(err)
	}
	interactionPlanner, err := interaction.NewPlanner(responderLLM)
	if err != nil {
		return nil, closeAll(err)
	}
	loop, err := agentloop.New(planner, executor, evaluator, responder, learning.NewRoutingRecorder(routeGraph))
	if err != nil {
		return nil, closeAll(err)
	}

	return &Runtime{
		loop:          loop,
		tools:         registry,
		conversations: conversation.NewStore(),
		interactions:  interactionPlanner,
		data:          data,
		tasks:         task.NewStore(),
		ctx:           ctx,
		approvals:     make(map[string]chan bool),
	}, nil
}

func newRoleLLM(common config.OpenAI, role config.Model) *llmclient.OpenAI {
	return llmclient.NewOpenAI(common.BaseURL, common.APIKey, role.Name, common.EmbeddingModel, role.EnableThinking)
}

func (r *Runtime) WorkspaceDB() storage.KnowledgeGraphReader {
	return r.data
}

func (r *Runtime) PlanInteraction(ctx context.Context, messages []string) (*interaction.Interaction, error) {
	if r == nil || r.interactions == nil {
		return nil, nil
	}
	result, err := r.interactions.PlanResult(ctx, messages)
	if err != nil {
		log.Printf("interaction_planner: error=%v", err)
		return nil, err
	}
	if result == nil {
		log.Printf("interaction_planner: source=nil")
		return nil, nil
	}
	if result.Interaction == nil {
		log.Printf("interaction_planner: source=%s reason=%q interaction=nil", result.Source, result.Reason)
		return nil, nil
	}
	log.Printf("interaction_planner: source=%s id=%s type=%s fields=%d reason=%q", result.Source, result.Interaction.ID, result.Interaction.Type, len(result.Interaction.Fields), result.Reason)
	return result.Interaction, nil
}

// RunTurn executes one user request in the named in-memory conversation.
func (r *Runtime) RunTurn(ctx context.Context, conversationID, text string) ([]string, error) {
	if !r.turnMu.TryLock() {
		return []string{MsgBusy}, nil
	}
	defer r.turnMu.Unlock()

	text = strings.TrimSpace(text)
	if text == "" {
		return nil, fmt.Errorf("runtime: empty user input")
	}
	turn := model.NewTurn(
		uuid.NewString(),
		conversationID,
		text,
		r.conversations.Context(conversationID),
		r.conversations.ContextArtifacts(conversationID),
	)
	if err := r.loop.Run(ctx, turn); err != nil {
		log.Printf("runtime: turn=%s conversation=%s error=%v", turn.ID, conversationID, err)
		return []string{friendlyRunError(err)}, nil
	}
	r.conversations.RememberContextArtifacts(conversationID, turn.ContextArtifactsSnapshot())
	r.conversations.AppendExchange(conversationID, text, turn.Answer)
	log.Printf("runtime: turn=%s conversation=%s status=%s actions=%d reason=%q", turn.ID, conversationID, turn.Status, len(turn.Steps), turn.TerminalReason)
	if trace := turn.TraceSummary(); trace != "" {
		log.Printf("run_trace: turn=%s\n%s", turn.ID, trace)
	}
	return turn.UserFacingMessages()
}

// SubmitAssistantInput creates a new task or resumes the active task when it
// is explicitly waiting for user input. Execution continues asynchronously.
func (r *Runtime) SubmitAssistantInput(activeTaskID, text string) (task.InputResult, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return task.InputResult{}, fmt.Errorf("runtime: empty user input")
	}

	activeTaskID = strings.TrimSpace(activeTaskID)
	if activeTaskID != "" {
		active, ok := r.tasks.Get(activeTaskID)
		if !ok {
			return task.InputResult{}, fmt.Errorf("task not found")
		}
		switch active.Status {
		case task.StatusWaitingInput:
			updated, err := r.tasks.PrepareInput(activeTaskID, text)
			if err != nil {
				return task.InputResult{}, err
			}
			go r.runTask(activeTaskID, text)
			return task.InputResult{Action: "task_continued", Task: updated}, nil
		case task.StatusRunning:
			return task.InputResult{}, fmt.Errorf("task is still running")
		case task.StatusWaitingApproval:
			return task.InputResult{}, fmt.Errorf("task is waiting for approval")
		}
	}

	created := r.tasks.Create(text, activeTaskID)
	go r.runTask(created.ID, text)
	return task.InputResult{Action: "task_created", Task: created}, nil
}

func (r *Runtime) SubmitInteraction(taskID, interactionID string, values map[string]any) (task.InputResult, error) {
	current, ok := r.tasks.Get(taskID)
	if !ok {
		return task.InputResult{}, fmt.Errorf("task not found")
	}
	if current.Status != task.StatusWaitingInput || current.PendingInteraction == nil {
		return task.InputResult{}, fmt.Errorf("task is not waiting for structured input")
	}
	if strings.TrimSpace(interactionID) != current.PendingInteraction.ID {
		return task.InputResult{}, fmt.Errorf("interaction does not match the pending task interaction")
	}
	message := interaction.RenderResponse(interaction.Response{Values: values}, current.PendingInteraction)
	return r.SubmitAssistantInput(taskID, message)
}

func (r *Runtime) Task(taskID string) (task.Snapshot, bool) {
	return r.tasks.Get(taskID)
}

func (r *Runtime) SubmitApproval(taskID, approvalID, decision string) (task.Snapshot, error) {
	decision = strings.TrimSpace(strings.ToLower(decision))
	updated, err := r.tasks.ResolveApproval(taskID, approvalID, decision)
	if err != nil {
		return task.Snapshot{}, err
	}
	r.approvalMu.Lock()
	waiter, ok := r.approvals[approvalID]
	if ok {
		delete(r.approvals, approvalID)
	}
	r.approvalMu.Unlock()
	if !ok {
		return task.Snapshot{}, fmt.Errorf("approval waiter not found")
	}
	waiter <- decision == "approved"
	close(waiter)
	return updated, nil
}

func (r *Runtime) runTask(taskID, text string) {
	ctx := r.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	if !r.turnMu.TryLock() {
		r.tasks.Finish(taskID, task.StatusFailed, []string{MsgBusy}, nil, "", MsgBusy)
		return
	}
	defer r.turnMu.Unlock()
	ctx = execution.WithApprovalHandler(ctx, func(ctx context.Context, action model.Action, policy toolcontract.Policy) error {
		approvalID := uuid.NewString()
		waiter := make(chan bool, 1)
		r.approvalMu.Lock()
		r.approvals[approvalID] = waiter
		r.approvalMu.Unlock()
		title := strings.TrimSpace(action.Goal)
		if title == "" {
			title = "执行高风险工具：" + action.Tool
		}
		if err := r.tasks.RequireApproval(taskID, task.Approval{
			ID:          approvalID,
			Title:       title,
			Description: strings.TrimSpace(policy.Summary),
		}); err != nil {
			return err
		}
		select {
		case approved := <-waiter:
			if !approved {
				return fmt.Errorf("rejected by user")
			}
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})

	turn := model.NewTurn(
		uuid.NewString(),
		"assistant",
		text,
		r.conversations.Context("assistant"),
		r.conversations.ContextArtifacts("assistant"),
	)
	turn.OnStepChanged = func(step *model.Step) {
		r.tasks.UpdateStep(taskID, taskStep(turn, step))
	}
	if err := r.loop.Run(ctx, turn); err != nil {
		log.Printf("runtime task: task=%s turn=%s error=%v", taskID, turn.ID, err)
		friendly := friendlyRunError(err)
		r.tasks.Finish(taskID, task.StatusFailed, []string{friendly}, nil, "", friendly)
		return
	}

	r.conversations.RememberContextArtifacts("assistant", turn.ContextArtifactsSnapshot())
	r.conversations.AppendExchange("assistant", text, turn.Answer)
	messages, err := turn.UserFacingMessages()
	if err != nil {
		friendly := friendlyRunError(err)
		r.tasks.Finish(taskID, task.StatusFailed, []string{friendly}, nil, "", friendly)
		return
	}
	form, _ := r.PlanInteraction(ctx, messages)
	status := task.StatusCompleted
	if turn.Status == model.TurnAwaitingInput || form != nil {
		status = task.StatusWaitingInput
	} else if turn.Status == model.TurnBlocked {
		status = task.StatusFailed
	}
	resultSummary := turn.Answer
	if status != task.StatusCompleted {
		resultSummary = ""
	}
	r.tasks.Finish(taskID, status, messages, form, resultSummary, "")
	log.Printf("runtime task: task=%s turn=%s status=%s steps=%d", taskID, turn.ID, status, len(turn.Steps))
}

func taskStep(turn *model.Turn, step *model.Step) task.Step {
	if turn == nil || step == nil {
		return task.Step{}
	}
	stepIndex := 0
	for i, candidate := range turn.Steps {
		if candidate == step {
			stepIndex = i + 1
			break
		}
	}
	id := fmt.Sprintf("%s:%d", turn.ID, stepIndex)
	summary := strings.TrimSpace(step.Summary)
	if summary == "" {
		summary = strings.TrimSpace(step.Assessment.Summary)
	}
	if summary == "" && step.Status != "running" {
		summary = clipTaskText(step.Observation.Result.CompactText(), 220)
	}
	summary = clipTaskText(summary, 180)
	out := task.Step{
		ID:          id,
		Kind:        strings.TrimSpace(step.Kind),
		Title:       strings.TrimSpace(step.Title),
		Tool:        strings.TrimSpace(step.Action.Tool),
		Status:      step.Status,
		Summary:     summary,
		Evaluation:  strings.TrimSpace(step.Assessment.Summary),
		StartedAt:   step.StartedAt,
		CompletedAt: step.CompletedAt,
	}
	if out.Title == "" {
		out.Title = strings.TrimSpace(step.Action.Goal)
	}
	if out.Title == "" {
		out.Title = out.Tool
	}
	if step.CompletedAt != nil && !step.StartedAt.IsZero() {
		out.DurationMS = step.CompletedAt.Sub(step.StartedAt).Milliseconds()
	}
	return out
}

func clipTaskText(value string, maxRunes int) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= maxRunes {
		return string(runes)
	}
	return string(runes[:maxRunes]) + "..."
}

func friendlyRunError(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	lower := strings.ToLower(msg)
	switch {
	case errors.Is(err, context.DeadlineExceeded), strings.Contains(lower, "timeout"), strings.Contains(lower, "deadline exceeded"):
		return "这次模型或工具调用超时了。请稍后重试，或把请求拆得更短一些。"
	case strings.Contains(lower, "connection refused"), strings.Contains(lower, "no such host"), strings.Contains(lower, "tls handshake timeout"):
		return "网络连接暂时不可用，模型或外部工具没有响应。"
	case strings.Contains(lower, "invalid arguments"), strings.Contains(lower, "decision json"):
		return "模型没有生成有效的结构化动作，本轮已停止。详细原因已写入 agent 日志。"
	default:
		return "本轮执行失败，详细原因已写入 agent 日志。"
	}
}

func (r *Runtime) Close() error {
	var err error
	if r.tools != nil {
		if closeErr := r.tools.Close(); closeErr != nil {
			err = errors.Join(err, closeErr)
		}
		r.tools = nil
	}
	if r.data != nil {
		if closeErr := r.data.Close(); closeErr != nil {
			err = errors.Join(err, closeErr)
		}
		r.data = nil
	}
	r.loop = nil
	return err
}
