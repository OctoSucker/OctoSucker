package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/OctoSucker/agent/internal/app"
	"github.com/OctoSucker/agent/internal/config"
)

func init() {
	log.SetFlags(log.Ltime | log.Llongfile)
}

func main() {
	workspaceDir := flag.String("workspace", "", "agent workspace directory (contains config.json)")
	enableCmd := flag.Bool("cmd", true, "enable local stdin REPL (set -cmd=false for non-interactive)")
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

	logDir := filepath.Join(wsRoot, "logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "octoplushttp:", err)
		os.Exit(1)
	}
	logPath := filepath.Join(logDir, "agent.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		fmt.Fprintln(os.Stderr, "octoplushttp:", err)
		os.Exit(1)
	}
	defer func() { _ = logFile.Close() }()
	log.SetOutput(logFile)
	fmt.Fprintf(os.Stderr, "agent log file: %s\n", logPath)

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

	if a.Telegram == nil && !*enableCmd {
		fmt.Fprintln(os.Stderr, "octoplushttp: need telegram bot_token or local cmd (-cmd=true)")
		os.Exit(1)
	}

	if *enableCmd {
		go func() {
			err := app.RunCmdREPL(ctx, a, logPath)
			if err != nil && !errors.Is(err, context.Canceled) {
				log.Printf("cmd repl: %v", err)
			}
		}()
	}

	if a.Telegram != nil {
		go func() {
			err := a.Telegram.RunPoll(ctx, func(ctx context.Context, chatID int64, text string) ([]string, error) {
				return a.RunInput(ctx, chatID, text)
			})
			if err != nil && !errors.Is(err, context.Canceled) {
				log.Printf("telegram poll: %v", err)
			}
		}()
	}

	<-ctx.Done()

}
