package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/OctoSucker/agent/internal/config"
	"github.com/OctoSucker/agent/internal/engine"
	"github.com/OctoSucker/agent/model"
	"github.com/OctoSucker/agent/pkg/telegram"
)

type App struct {
	Dispatcher *engine.Dispatcher
	Telegram   *telegram.Ingress
	data       *model.AgentDB
	turnMu     sync.Mutex
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
	var tg *telegram.Ingress
	if strings.TrimSpace(cfg.Telegram.BotToken) != "" {
		var errTg error
		tg, errTg = telegram.NewIngress(cfg.Telegram.BotToken, cfg.Telegram.DefaultChatID, cfg.Telegram.AllowedChatIDs)
		if errTg != nil {
			if cerr := data.Close(); cerr != nil {
				errTg = errors.Join(errTg, fmt.Errorf("close data db: %w", cerr))
			}
			return nil, fmt.Errorf("octoplus: telegram ingress: %w", errTg)
		}
	}
	a := &App{
		Dispatcher: d,
		data:       data,
		Telegram:   tg,
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
