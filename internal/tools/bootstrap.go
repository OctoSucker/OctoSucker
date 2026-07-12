package tools

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/OctoSucker/octosucker/config"
	cronjobbuiltin "github.com/OctoSucker/octosucker/internal/tools/builtin/cronjob"
	execbuiltin "github.com/OctoSucker/octosucker/internal/tools/builtin/exec"
	skillsbuiltin "github.com/OctoSucker/octosucker/internal/tools/builtin/skills"
	telegrambuiltin "github.com/OctoSucker/octosucker/internal/tools/builtin/telegram"
	thinkerbuiltin "github.com/OctoSucker/octosucker/internal/tools/builtin/thinker"
	mcpstore "github.com/OctoSucker/octosucker/internal/tools/mcp"
	opencliprovider "github.com/OctoSucker/octosucker/internal/tools/opencli"
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

func (r *Registry) registerBuiltins(ctx context.Context, workspaceRoot string, execCfg config.Exec, telegramCfg config.Telegram, openCLICfg config.OpenCLI, skillsDir string, embedLLM *llmclient.OpenAI) error {
	if err := r.registerExecProvider(execCfg); err != nil {
		return err
	}
	if err := r.registerTelegramProvider(telegramCfg); err != nil {
		return err
	}
	if err := r.registerSkillsProvider(skillsDir); err != nil {
		return err
	}
	if err := r.registerOpenCLIProvider(ctx, openCLICfg); err != nil {
		return err
	}
	if err := r.registerThinkerProvider(embedLLM); err != nil {
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

func (r *Registry) registerOpenCLIProvider(ctx context.Context, cfg config.OpenCLI) error {
	if strings.TrimSpace(cfg.Command) == "" {
		return nil
	}
	provider, err := opencliprovider.NewProvider(ctx, opencliprovider.Options{
		Command:  cfg.Command,
		Commands: cfg.Commands,
		Timeout:  time.Duration(cfg.CommandTimeoutSec) * time.Second,
	})
	if err != nil {
		return fmt.Errorf("tool registry: opencli provider: %w", err)
	}
	return r.addProvider(provider)
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

func (r *Registry) registerSkillsProvider(skillsDir string) error {
	skillRunner, err := skillsbuiltin.NewRunner(skillsDir)
	if err != nil {
		return fmt.Errorf("tool registry: skills runner: %w", err)
	}
	if err := r.addProvider(skillRunner); err != nil {
		return err
	}
	r.skillsProvider = skillRunner
	return nil
}

func (r *Registry) registerThinkerProvider(embedLLM *llmclient.OpenAI) error {
	thinkerRunner, err := thinkerbuiltin.NewRunner(embedLLM)
	if err != nil {
		return fmt.Errorf("tool registry: thinker builtin: %w", err)
	}
	return r.addProvider(thinkerRunner)
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
