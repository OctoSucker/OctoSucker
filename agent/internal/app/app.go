package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/OctoSucker/agent/internal/config"
	"github.com/OctoSucker/agent/internal/engine"
	"github.com/OctoSucker/agent/internal/store"
	"github.com/OctoSucker/agent/pkg/telegram"
)

type App struct {
	Dispatcher *engine.Dispatcher
	Telegram   *telegram.Ingress
	dataDB     *sql.DB
}

func NewFromWorkspace(ctx context.Context, workspaceRoot string, cfg *config.Workspace) (*App, error) {
	if cfg == nil {
		return nil, fmt.Errorf("octoplus: workspace config required")
	}
	dataDB, err := store.OpenAgentDB(workspaceRoot)
	if err != nil {
		return nil, fmt.Errorf("octoplus: sqlite: %w", err)
	}

	d, err := engine.NewDispatcher(ctx, cfg.MCPEndpoint, cfg.OpenAI, cfg.Exec, cfg.Telegram, cfg.SkillsDir, dataDB)
	if err != nil {
		if cerr := dataDB.Close(); cerr != nil {
			err = errors.Join(err, fmt.Errorf("close data db: %w", cerr))
		}
		return nil, fmt.Errorf("octoplus: dispatcher: %w", err)
	}
	telegram, err := telegram.NewIngress(cfg.Telegram.BotToken, cfg.Telegram.DefaultChatID, cfg.Telegram.AllowedChatIDs)
	if err != nil {
		if cerr := dataDB.Close(); cerr != nil {
			err = errors.Join(err, fmt.Errorf("close data db: %w", cerr))
		}
		return nil, fmt.Errorf("octoplus: telegram ingress: %w", err)
	}
	a := &App{
		Dispatcher: d,
		dataDB:     dataDB,
		Telegram:   telegram,
	}

	return a, nil
}

func (a *App) Close() error {
	var err error
	if a.dataDB != nil {
		if cerr := a.dataDB.Close(); cerr != nil {
			err = errors.Join(err, fmt.Errorf("close data db: %w", cerr))
		}
		a.dataDB = nil
	}
	if a.Dispatcher != nil {
		a.Dispatcher = nil
	}
	return err
}
