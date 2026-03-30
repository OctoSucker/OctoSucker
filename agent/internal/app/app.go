package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/OctoSucker/agent/internal/config"
	"github.com/OctoSucker/agent/internal/engine"
	"github.com/OctoSucker/agent/model"
	"github.com/OctoSucker/agent/pkg/telegram"
)

type App struct {
	Dispatcher *engine.Dispatcher
	Telegram   *telegram.Ingress
	data       *model.AgentDB
}

func NewFromWorkspace(ctx context.Context, workspaceRoot string, cfg *config.Workspace) (*App, error) {
	if cfg == nil {
		return nil, fmt.Errorf("octoplus: workspace config required")
	}
	data, err := model.OpenAgentDB(workspaceRoot)
	if err != nil {
		return nil, fmt.Errorf("octoplus: sqlite: %w", err)
	}

	d, err := engine.NewDispatcher(ctx, cfg.MCPEndpoint, cfg.OpenAI, cfg.Exec, cfg.Telegram, cfg.SkillsDir, data)
	if err != nil {
		if cerr := data.Close(); cerr != nil {
			err = errors.Join(err, fmt.Errorf("close data db: %w", cerr))
		}
		return nil, fmt.Errorf("octoplus: dispatcher: %w", err)
	}
	telegram, err := telegram.NewIngress(cfg.Telegram.BotToken, cfg.Telegram.DefaultChatID, cfg.Telegram.AllowedChatIDs)
	if err != nil {
		if cerr := data.Close(); cerr != nil {
			err = errors.Join(err, fmt.Errorf("close data db: %w", cerr))
		}
		return nil, fmt.Errorf("octoplus: telegram ingress: %w", err)
	}
	a := &App{
		Dispatcher: d,
		data:       data,
		Telegram:   telegram,
	}

	return a, nil
}

func (a *App) Close() error {
	var err error
	if a.data != nil {
		if cerr := a.data.Close(); cerr != nil {
			err = errors.Join(err, fmt.Errorf("close data db: %w", cerr))
		}
		a.data = nil
	}
	if a.Dispatcher != nil {
		a.Dispatcher = nil
	}
	return err
}
