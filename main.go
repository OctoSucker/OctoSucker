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

	"github.com/OctoSucker/octosucker/app"
	"github.com/OctoSucker/octosucker/config"
	"github.com/OctoSucker/octosucker/pkg/repl"
	"github.com/OctoSucker/octosucker/pkg/workspacelog"
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
		fmt.Fprintln(os.Stderr, "octosucker:", err)
		os.Exit(1)
	}

	cfg, err := config.LoadWorkspace(wsRoot)
	if err != nil {
		fmt.Fprintln(os.Stderr, "octosucker:", err)
		os.Exit(1)
	}

	logFile, logPath, err := workspacelog.OpenFile(wsRoot, "agent.log")
	if err != nil {
		fmt.Fprintln(os.Stderr, "octosucker:", err)
		os.Exit(1)
	}
	defer func() { _ = logFile.Close() }()
	log.SetOutput(logFile)
	fmt.Fprintf(os.Stderr, "agent log file: %s\n", logPath)

	a, err := app.NewFromWorkspace(ctx, wsRoot, cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "octosucker:", err)
		os.Exit(1)
	}
	defer func() {
		if err := a.Close(); err != nil {
			log.Printf("app close: %v", err)
		}
	}()

	if a.Telegram == nil && !*enableCmd {
		fmt.Fprintln(os.Stderr, "octosucker: need telegram bot_token or local cmd (-cmd=true)")
		os.Exit(1)
	}

	if *enableCmd {
		go func() {
			err := repl.RunCmdREPL(ctx, func(ctx context.Context, text string) ([]string, error) {
				return a.RunInputFromLocal(ctx, text)
			}, logPath)
			if err != nil && !errors.Is(err, context.Canceled) {
				log.Printf("cmd repl: %v", err)
			}
		}()
	}

	if a.Telegram != nil {
		go func() {
			err := a.Telegram.RunPoll(ctx, func(ctx context.Context, chatID int64, text string) ([]string, error) {
				return a.RunInputFromTelegram(ctx, chatID, text)
			})
			if err != nil && !errors.Is(err, context.Canceled) {
				log.Printf("telegram poll: %v", err)
			}
		}()
	}

	<-ctx.Done()
}
