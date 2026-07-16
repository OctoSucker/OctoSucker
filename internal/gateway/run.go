package gateway

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/OctoSucker/octosucker/config"
	"github.com/OctoSucker/octosucker/internal/ingress/telegram"
)

// Run requires at least one configured service ingress.
// It starts the corresponding transports, then blocks until ctx is canceled.
func Run(ctx context.Context, agent Agent, cfg *config.Workspace) error {
	if cfg == nil {
		return fmt.Errorf("gateway: workspace config required")
	}

	adminListen := strings.TrimSpace(cfg.HTTP.Listen)
	var tg *telegram.Ingress
	if strings.TrimSpace(cfg.Telegram.BotToken) != "" {
		var err error
		tg, err = telegram.NewIngress(cfg.Telegram.BotToken, cfg.Telegram.DefaultChatID, cfg.Telegram.AllowedChatIDs)
		if err != nil {
			return fmt.Errorf("telegram ingress: %w", err)
		}
	}

	if tg == nil && adminListen == "" {
		return fmt.Errorf("need telegram bot_token or http.listen in config.json")
	}

	var adminSrv *http.Server
	if adminListen != "" {
		h, err := adminHTTPHandler(agent)
		if err != nil {
			return err
		}
		adminSrv = &http.Server{Addr: adminListen, Handler: h}
		go func() {
			fmt.Fprintf(os.Stderr, "admin web: http://%s\n", adminListen)
			if err := adminSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Printf("admin web: %v", err)
			}
		}()
	}
	defer func() {
		if adminSrv == nil {
			return
		}
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := adminSrv.Shutdown(shutdownCtx); err != nil {
			log.Printf("admin web shutdown: %v", err)
		}
	}()

	if tg != nil {
		go func() {
			err := tg.RunPoll(ctx, func(ctx context.Context, chatID int64, text string) ([]string, error) {
				return agent.RunTurn(ctx, fmt.Sprintf("telegram:%d", chatID), text)
			})
			if err != nil && !errors.Is(err, context.Canceled) {
				log.Printf("telegram poll: %v", err)
			}
		}()
	}

	<-ctx.Done()
	return nil
}
