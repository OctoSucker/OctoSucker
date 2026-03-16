package runtime

import (
	"context"
	"errors"
	"log"
	"time"

	mcp "github.com/OctoSucker/octosucker-mcp"
	tools "github.com/OctoSucker/octosucker-tools"
	"github.com/OctoSucker/octosucker/agent/llm"
	"github.com/OctoSucker/octosucker/agent/memory"
	"github.com/OctoSucker/octosucker/agent/planner"
	capregistry "github.com/OctoSucker/octosucker/capability/registry"
	capworkflow "github.com/OctoSucker/octosucker/capability/workflow"
	"github.com/OctoSucker/octosucker/config"
)

type AgentRuntime struct {
	llmClient          llm.ToolChatClient
	toolRegistry       *tools.ToolRegistry
	mcpRegistry        *mcp.MCPRegistry
	capabilityRegistry *capregistry.Registry
	planner            *planner.Planner
	selector           *planner.Selector
	generator          *planner.Generator
	memory             memory.VectorMemory
	taskQueue          chan *Task
	maxReActIterations int
	maxToolRetries     int
	toolTimeout        time.Duration
	taskTimeout        time.Duration
	systemPrompt       string
	configPath         string
}

func (r *AgentRuntime) ExecuteTool(ctx context.Context, name string, argumentsJSON string) (interface{}, error) {
	_, err := r.toolRegistry.GetTool(name)
	if err == nil {
		return r.toolRegistry.ExecuteTool(ctx, name, argumentsJSON)
	}
	if r.mcpRegistry != nil {
		return r.mcpRegistry.ExecuteTool(ctx, name, argumentsJSON)
	}
	return nil, err
}

func NewAgentRuntime(
	ctx context.Context,
	configPath string,
	llmCfg *config.LLMConfig,
	reactCfg *config.ReActConfig,
	toolProviderConfigs map[string]map[string]interface{},
	mcpServers []map[string]interface{},
	systemPrompt string,
	workflowTemplates []capworkflow.WorkflowTemplate,
) (*AgentRuntime, error) {
	if reactCfg == nil {
		return nil, errors.New("react config is required")
	}
	llmClient := llm.NewLLMClient(llmCfg.BaseURL, llmCfg.APIKey, llmCfg.Model, reactCfg.EmbeddingModel)
	mem, err := memory.NewVectorMemory(reactCfg.MemoryPath, llmClient)
	if err != nil {
		return nil, err
	}

	rt := &AgentRuntime{
		llmClient:          llmClient,
		memory:             mem,
		taskQueue:          make(chan *Task, 100),
		maxReActIterations: reactCfg.MaxReActIterations,
		maxToolRetries:     reactCfg.MaxToolRetries,
		toolTimeout:        time.Duration(reactCfg.ToolTimeoutMs) * time.Millisecond,
		taskTimeout:        time.Duration(reactCfg.TaskTimeoutSec) * time.Second,
		systemPrompt:       systemPrompt,
		configPath:         configPath,
	}

	rt.capabilityRegistry = capregistry.NewRegistry()

	rt.toolRegistry = tools.NewToolRegistry()
	if toolProviderConfigs == nil {
		toolProviderConfigs = make(map[string]map[string]interface{})
	}
	if failed := tools.LoadAllToolProviders(rt.toolRegistry, rt, toolProviderConfigs, rt.SubmitTask); len(failed) > 0 {
		log.Printf("Warning: %d tool provider(s) failed to load:", len(failed))
		for name, err := range failed {
			log.Printf("  - %s: %v", name, err)
		}
	}

	rt.capabilityRegistry.RegisterToolCapabilities(rt.toolRegistry.GetAllTools())

	capworkflow.ResolveTemplateTools(
		ctx,
		workflowTemplates,
		rt.capabilityRegistry,
		llmClient,
		llm.CosineSimilarity,
		capworkflow.BindingCachePath(reactCfg.WorkflowBindingCachePath),
	)
	capworkflow.RegisterSkillWorkflows(rt.capabilityRegistry, workflowTemplates)
	rt.capabilityRegistry.RefreshEntryCapabilities()

	rt.planner = planner.NewPlannerWithLLM(rt.capabilityRegistry, llmClient)
	rt.selector = planner.NewSelector(rt.capabilityRegistry, llmClient, reactCfg.SelectorSemanticCandidateLimit)
	if reactCfg.SelectorEmbeddingCacheTTLSec > 0 {
		rt.selector.SetEmbeddingCacheTTL(time.Duration(reactCfg.SelectorEmbeddingCacheTTLSec) * time.Second)
	}
	rt.generator = planner.NewGenerator(llmClient)

	rt.mcpRegistry = mcp.NewMCPRegistry()
	if failedMCP := mcp.LoadAllMCPProviders(rt.mcpRegistry, mcpServers); len(failedMCP) > 0 {
		log.Printf("Warning: MCP provider(s) failed to load:")
		for name, err := range failedMCP {
			log.Printf("  - %s: %v", name, err)
		}
	}

	return rt, nil
}

func (r *AgentRuntime) Start(ctx context.Context, taskInputs []string) error {
	go r.runTaskQueueProcessor(ctx)
	if len(taskInputs) > 0 {
		for _, input := range taskInputs {
			if input == "" {
				continue
			}
			if err := r.SubmitTask(input); err != nil {
				log.Printf("Failed to submit startup task: %v", err)
			}
		}
	}
	<-ctx.Done()
	log.Printf("AgentRuntime context cancelled, shutting down...")
	return ctx.Err()
}

func (r *AgentRuntime) runTaskQueueProcessor(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case task := <-r.taskQueue:
			loopCtx := ctx
			var cancel context.CancelFunc
			if r.taskTimeout > 0 {
				loopCtx, cancel = context.WithTimeout(ctx, r.taskTimeout)
			}
			r.runReActLoop(loopCtx, task)
			if cancel != nil {
				cancel()
			}
		}
	}
}
