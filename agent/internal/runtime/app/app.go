package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/OctoSucker/agent/internal/config"
	"github.com/OctoSucker/agent/internal/runtime/engine"
	"github.com/OctoSucker/agent/internal/runtime/store"
	"github.com/OctoSucker/agent/pkg/mcpclient"
	"github.com/OctoSucker/agent/pkg/telegram"
)

type App struct {
	Dispatcher *engine.Dispatcher
	Telegram   *telegram.Ingress
	dataDB     *sql.DB
	shutdown   func()
}

func NewFromWorkspace(ctx context.Context, workspaceRoot string, cfg *config.Workspace) (*App, error) {
	if cfg == nil {
		return nil, fmt.Errorf("octoplus: workspace config required")
	}

	mcpRouter, shutdown, err := mcpclient.NewMCPRouter(ctx, cfg.MCPEndpoint)
	if err != nil {
		return nil, err
	}

	dataDB, err := store.OpenAgentDB(workspaceRoot)
	if err != nil {
		shutdown()
		return nil, fmt.Errorf("octoplus: sqlite: %w", err)
	}

	d, err := engine.NewDispatcher(ctx, mcpRouter, cfg.OpenAI, dataDB)
	if err != nil {
		if cerr := dataDB.Close(); cerr != nil {
			err = errors.Join(err, fmt.Errorf("close data db: %w", cerr))
		}
		shutdown()
		return nil, fmt.Errorf("octoplus: dispatcher: %w", err)
	}
	telegram, err := telegram.NewIngress(cfg.Telegram.BotToken, cfg.Telegram.DefaultChatID, cfg.Telegram.AllowedChatIDs)
	if err != nil {
		if cerr := dataDB.Close(); cerr != nil {
			err = errors.Join(err, fmt.Errorf("close data db: %w", cerr))
		}
		shutdown()
		return nil, fmt.Errorf("octoplus: telegram ingress: %w", err)
	}
	a := &App{
		Dispatcher: d,
		dataDB:     dataDB,
		shutdown:   shutdown,
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
	if a.shutdown != nil {
		a.shutdown()
		a.shutdown = nil
	}
	return err
}
