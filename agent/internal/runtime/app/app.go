package app

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"

	"github.com/OctoSucker/agent/internal/config"
	"github.com/OctoSucker/agent/internal/runtime/engine"
	"github.com/OctoSucker/agent/internal/runtime/store"
	"github.com/OctoSucker/agent/pkg/mcpclient"
	"github.com/OctoSucker/agent/pkg/telegram"
)

type App struct {
	Dispatcher *engine.Dispatcher

	Telegram   *telegram.Ingress
	HTTPServer *http.Server
	dataDB     *sql.DB
	shutdown   func()
}

func NewFromWorkspace(ctx context.Context, workspaceRoot string, cfg *config.Workspace) (*App, error) {
	if cfg == nil {
		return nil, fmt.Errorf("octoplus: workspace config required")
	}

	mcpRouter, shutdown, err := mcpclient.ConnectForApp(ctx, cfg.MCPEndpoint)
	if err != nil {
		return nil, err
	}

	caps, err := mcpRouter.ListCapabilities(ctx)
	if err != nil {
		shutdown()
		return nil, err
	}
	if len(caps) == 0 {
		shutdown()
		return nil, fmt.Errorf("octoplus: MCP exposed no tools; ensure list_tools returns tools")
	}

	dataDB, err := store.OpenAgentDB(workspaceRoot)
	if err != nil {
		shutdown()
		return nil, fmt.Errorf("octoplus: sqlite: %w", err)
	}

	d := engine.NewDispatcher(500, mcpRouter, caps, cfg.OpenAI, dataDB)
	telegram, err := telegram.NewIngress(cfg.Telegram.BotToken, cfg.Telegram.DefaultChatID, cfg.Telegram.AllowedChatIDs)
	if err != nil {
		_ = dataDB.Close()
		shutdown()
		return nil, fmt.Errorf("octoplus: telegram ingress: %w", err)
	}
	a := &App{
		Dispatcher: d,
		dataDB:     dataDB,
		shutdown:   shutdown,
		Telegram:   telegram,
	}
	a.HTTPServer = &http.Server{Addr: cfg.HTTP.Listen, Handler: a.HTTPHandler()}

	return a, nil
}

func (a *App) Close() {
	if a != nil && a.dataDB != nil {
		_ = a.dataDB.Close()
		a.dataDB = nil
	}
	if a != nil && a.shutdown != nil {
		a.shutdown()
		a.shutdown = nil
	}
}
