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
	turnMu        sync.Mutex
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
