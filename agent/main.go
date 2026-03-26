package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/OctoSucker/agent/internal/config"
	"github.com/OctoSucker/agent/internal/runtime/app"
)

func init() {
	log.SetFlags(log.Ltime | log.Llongfile)
}

func main() {
	workspaceDir := flag.String("workspace", "", "agent workspace directory (contains config.json)")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	wsRoot, err := config.ResolveAndEnsureDir(*workspaceDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "octoplushttp:", err)
		os.Exit(1)
	}

	cfg, err := config.LoadWorkspace(wsRoot)
	if err != nil {
		fmt.Fprintln(os.Stderr, "octoplushttp:", err)
		os.Exit(1)
	}
	a, err := app.NewFromWorkspace(ctx, wsRoot, cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "octoplushttp:", err)
		os.Exit(1)
	}
	defer func() {
		if err := a.Close(); err != nil {
			log.Printf("app close: %v", err)
		}
	}()

	if a.Telegram != nil {
		go func() {
			err := a.Telegram.RunPoll(ctx, func(ctx context.Context, chatID int64, text string) (string, error) {
				return a.RunInput(ctx, chatID, text)
			})
			if err != nil && !errors.Is(err, context.Canceled) {
				log.Printf("telegram poll: %v", err)
			}
		}()
	}

	<-ctx.Done()

}
