package agent

import (
	"context"
	"log"
	"time"

	skills "github.com/OctoSucker/octosucker-skills"
	tools "github.com/OctoSucker/octosucker-tools"
	"github.com/OctoSucker/octosucker/agent/llm"
	"github.com/OctoSucker/octosucker/agent/memory"
	"github.com/OctoSucker/octosucker/config"
)

type Agent struct {
	llmClient          *llm.LLMClient
	toolRegistry       *tools.ToolRegistry
	skillRegistry      *skills.Registry
	memory             memory.VectorMemory
	taskQueue          chan *Task
	maxReActIterations int
	taskTimeout        time.Duration
	systemPrompt       string
	configPath         string
}

func NewAgent(
	ctx context.Context,
	configPath string,
	llmCfg *config.LLMConfig,
	reactCfg *config.ReActConfig,
	toolProviderConfigs map[string]map[string]interface{},
	skillsDirs []string,
	systemPrompt string,
) (*Agent, error) {

	llmClient := llm.NewLLMClient(llmCfg.BaseURL, llmCfg.APIKey, llmCfg.Model, reactCfg.EmbeddingModel)
	mem, err := memory.NewVectorMemory(reactCfg.MemoryPath, llmClient)
	if err != nil {
		return nil, err
	}
	agent := &Agent{
		llmClient:          llmClient,
		toolRegistry:       tools.NewToolRegistry(),
		skillRegistry:      skills.NewRegistry(),
		memory:             mem,
		taskQueue:          make(chan *Task, 100),
		maxReActIterations: reactCfg.MaxReActIterations,
		taskTimeout:        time.Duration(reactCfg.TaskTimeoutSec) * time.Second,
		systemPrompt:       systemPrompt,
		configPath:         configPath,
	}

	failed := tools.LoadAllToolProviders(agent.toolRegistry, agent, toolProviderConfigs)
	if len(failed) > 0 {
		log.Printf("Warning: %d tool provider(s) failed to load:", len(failed))
		for name, err := range failed {
			log.Printf("  - %s: %v", name, err)
		}
	}

	if len(skillsDirs) > 0 {
		if err := agent.skillRegistry.LoadFromDirs(skillsDirs); err != nil {
			log.Printf("Warning: failed to load skills from %v: %v", skillsDirs, err)
		}
	}
	return agent, nil
}

func (a *Agent) Start(ctx context.Context, taskInputs []string) error {
	go a.runTaskQueueProcessor(ctx)

	if len(taskInputs) > 0 {
		a.submitInitializationTasks(taskInputs)
	}
	<-ctx.Done()
	log.Printf("Agent context cancelled, shutting down...")
	return ctx.Err()
}

func (a *Agent) runTaskQueueProcessor(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case task := <-a.taskQueue:
			loopCtx := ctx
			var cancel context.CancelFunc
			if a.taskTimeout > 0 {
				loopCtx, cancel = context.WithTimeout(ctx, a.taskTimeout)
			}
			go func(t *Task) {
				if cancel != nil {
					defer cancel()
				}
				a.runReActLoop(loopCtx, t)
			}(task)
		}
	}
}

func (a *Agent) GetConfigPath() string {
	return a.configPath
}

func (a *Agent) SkillRegistry() *skills.Registry {
	return a.skillRegistry
}
