// Package app wires the tool-planning workspace agent: Dispatcher, optional Telegram, RunInput.
package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/OctoSucker/octosucker/config"
	"github.com/OctoSucker/octosucker/engine"
	"github.com/OctoSucker/octosucker/pkg/telegram"
	"github.com/OctoSucker/octosucker/store"
)

// App is the tool-planning agent: Dispatcher, optional Telegram, workspace SQLite.
type App struct {
	Dispatcher *engine.Dispatcher
	Telegram   *telegram.Ingress
	data       *store.DB
	turnMu     sync.Mutex
}

// NewFromWorkspace builds the tool-planning app from workspace config (MCP, OpenAI, exec sandbox, etc.).
func NewFromWorkspace(ctx context.Context, workspaceRoot string, cfg *config.Workspace) (*App, error) {
	if cfg == nil {
		return nil, fmt.Errorf("octosucker: workspace config required")
	}
	data, err := store.Open(workspaceRoot)
	if err != nil {
		return nil, fmt.Errorf("octosucker: sqlite: %w", err)
	}

	d, err := engine.NewDispatcher(ctx, cfg.MCPEndpoint, cfg.OpenAI, cfg.Exec, cfg.Telegram, cfg.SkillsDir, data)
	if err != nil {
		if cerr := data.Close(); cerr != nil {
			err = errors.Join(err, fmt.Errorf("close data db: %w", cerr))
		}
		return nil, fmt.Errorf("octosucker: dispatcher: %w", err)
	}
	var tg *telegram.Ingress
	if strings.TrimSpace(cfg.Telegram.BotToken) != "" {
		var errTg error
		tg, errTg = telegram.NewIngress(cfg.Telegram.BotToken, cfg.Telegram.DefaultChatID, cfg.Telegram.AllowedChatIDs)
		if errTg != nil {
			if cerr := data.Close(); cerr != nil {
				errTg = errors.Join(errTg, fmt.Errorf("close data db: %w", cerr))
			}
			return nil, fmt.Errorf("octosucker: telegram ingress: %w", errTg)
		}
	}
	a := &App{
		Dispatcher: d,
		data:       data,
		Telegram:   tg,
	}

	return a, nil
}

// Close closes the workspace database and drops dispatcher references.
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
