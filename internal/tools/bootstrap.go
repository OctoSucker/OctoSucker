package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/OctoSucker/octosucker/config"
	"github.com/OctoSucker/octosucker/internal/storage"
	catalogbuiltin "github.com/OctoSucker/octosucker/internal/tools/builtin/catalog"
	cronjobbuiltin "github.com/OctoSucker/octosucker/internal/tools/builtin/cronjob"
	execbuiltin "github.com/OctoSucker/octosucker/internal/tools/builtin/exec"
	marketintelbuiltin "github.com/OctoSucker/octosucker/internal/tools/builtin/marketintel"
	skillsbuiltin "github.com/OctoSucker/octosucker/internal/tools/builtin/skills"
	telegrambuiltin "github.com/OctoSucker/octosucker/internal/tools/builtin/telegram"
	thinkerbuiltin "github.com/OctoSucker/octosucker/internal/tools/builtin/thinker"
	mcpstore "github.com/OctoSucker/octosucker/internal/tools/mcp"
	"github.com/OctoSucker/octosucker/internal/tools/skillcli"
	"github.com/OctoSucker/octosucker/pkg/llmclient"
)

func (r *Registry) addProvider(p Provider) error {
	if p == nil {
		return fmt.Errorf("tool registry: provider is nil")
	}
	id, _ := p.Name()
	if id == "" {
		return fmt.Errorf("tool registry: provider name is empty")
	}
	if _, exists := r.providersByName[id]; exists {
		return fmt.Errorf("tool registry: duplicate provider %q", id)
	}
	r.providersByName[id] = p
	return nil
}

func validateWorkspaceRoot(workspaceRoot string) (string, error) {
	root := strings.TrimSpace(workspaceRoot)
	if root == "" {
		return "", fmt.Errorf("tool registry: workspace root is required")
	}
	return root, nil
}

func (r *Registry) registerBuiltins(workspaceRoot string, openAI config.OpenAI, data *storage.DB, execCfg config.Exec, telegramCfg config.Telegram, skillsDir string, embedLLM *llmclient.OpenAI) error {
	if err := r.registerExecProvider(execCfg); err != nil {
		return err
	}
	if err := r.registerTelegramProvider(telegramCfg); err != nil {
		return err
	}
	if err := r.registerSkillsProvider(workspaceRoot, openAI, skillsDir); err != nil {
		return err
	}
	if err := r.registerCatalogProvider(embedLLM); err != nil {
		return err
	}
	if err := r.registerThinkerProvider(embedLLM); err != nil {
		return err
	}
	if err := r.registerMarketIntelProvider(embedLLM); err != nil {
		return err
	}
	if err := r.registerCronjobProvider(workspaceRoot); err != nil {
		return err
	}
	if err := r.addProvider(newIntrospectionBackend(r)); err != nil {
		return err
	}
	return nil
}

func (r *Registry) registerExecProvider(execCfg config.Exec) error {
	execB, err := execbuiltin.NewRunner(execCfg)
	if err != nil {
		return err
	}
	return r.addProvider(execB)
}

func (r *Registry) registerTelegramProvider(cfg config.Telegram) error {
	if strings.TrimSpace(cfg.BotToken) == "" {
		return nil
	}
	tg, err := telegrambuiltin.NewRunner(cfg)
	if err != nil {
		return fmt.Errorf("tool registry: telegram builtin: %w", err)
	}
	return r.addProvider(tg)
}

func (r *Registry) registerSkillsProvider(workspaceRoot string, openAI config.OpenAI, skillsDir string) error {
	skillRunner, err := skillsbuiltin.NewRunner(skillsDir)
	if err != nil {
		return fmt.Errorf("tool registry: skills runner: %w", err)
	}
	if err := r.addProvider(skillRunner); err != nil {
		return err
	}
	r.skillsProvider = skillRunner
	for _, skill := range skillRunner.AllSkills() {
		if skill.CLIPlugin == nil {
			continue
		}
		provider, err := skillcli.NewProvider(workspaceRoot, openAI, skill)
		if err != nil {
			return fmt.Errorf("tool registry: skill cli plugin %q: %w", skill.Name, err)
		}
		if err := r.addProvider(provider); err != nil {
			return err
		}
	}
	return nil
}

func (r *Registry) registerCatalogProvider(embedLLM *llmclient.OpenAI) error {
	catalogRunner, err := catalogbuiltin.NewRunner(embedLLM)
	if err != nil {
		return fmt.Errorf("tool registry: catalog runner: %w", err)
	}
	return r.addProvider(catalogRunner)
}

func (r *Registry) registerThinkerProvider(embedLLM *llmclient.OpenAI) error {
	thinkerRunner, err := thinkerbuiltin.NewRunner(embedLLM)
	if err != nil {
		return fmt.Errorf("tool registry: thinker builtin: %w", err)
	}
	return r.addProvider(thinkerRunner)
}

func (r *Registry) registerMarketIntelProvider(llm *llmclient.OpenAI) error {
	runner, err := marketintelbuiltin.NewRunner(llm)
	if err != nil {
		return fmt.Errorf("tool registry: marketintel builtin: %w", err)
	}
	return r.addProvider(runner)
}

func (r *Registry) registerCronjobProvider(workspaceRoot string) error {
	workspaceRoot, err := validateWorkspaceRoot(workspaceRoot)
	if err != nil {
		return fmt.Errorf("tool registry: cronjob builtin: %w", err)
	}
	cj, err := cronjobbuiltin.NewRunner(workspaceRoot)
	if err != nil {
		return fmt.Errorf("tool registry: cronjob builtin: %w", err)
	}
	return r.addProvider(cj)
}

func (r *Registry) registerMCPProviders(ctx context.Context, endpoints []string) error {
	for _, ep := range endpoints {
		sess, err := mcpstore.NewRemoteSession(ctx, ep)
		if err != nil {
			return fmt.Errorf("tool registry: connect mcp endpoint %q: %w", ep, err)
		}
		if err := r.addProvider(sess); err != nil {
			return err
		}
	}
	return nil
}
