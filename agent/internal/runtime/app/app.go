package app

import (
	"context"
	"fmt"
	"net/http"

	"github.com/OctoSucker/agent/internal/config"
	"github.com/OctoSucker/agent/internal/runtime/engine"
	"github.com/OctoSucker/agent/pkg/mcpclient"
	"github.com/OctoSucker/agent/pkg/telegram"
)

type App struct {
	Dispatcher *engine.Dispatcher

	Telegram   *telegram.Ingress
	HTTPServer *http.Server
	shutdown   func()
}

func NewFromWorkspace(ctx context.Context, cfg *config.Workspace) (*App, error) {
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
	d := engine.NewDispatcher(500, mcpRouter, caps, cfg.OpenAI)
	telegram, err := telegram.NewIngress(cfg.Telegram.BotToken, cfg.Telegram.DefaultChatID, cfg.Telegram.AllowedChatIDs)
	if err != nil {
		return nil, fmt.Errorf("octoplus: telegram ingress: %w", err)
	}
	a := &App{
		Dispatcher: d,
		shutdown:   shutdown,
		Telegram:   telegram,
	}
	a.HTTPServer = &http.Server{Addr: cfg.HTTP.Listen, Handler: a.HTTPHandler()}

	return a, nil
}

func (a *App) Close() {
	if a != nil && a.shutdown != nil {
		a.shutdown()
		a.shutdown = nil
	}
}
