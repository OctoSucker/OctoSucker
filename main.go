package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

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
	webAddr := flag.String("web", "", "if set (e.g. 127.0.0.1:8090), serve web admin UI: chat + knowledge graph")
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

	var adminSrv *http.Server
	if *webAddr != "" {
		adminHandler, err := a.AdminHandler()
		if err != nil {
			fmt.Fprintln(os.Stderr, "octosucker:", err)
			os.Exit(1)
		}
		adminSrv = &http.Server{Addr: *webAddr, Handler: adminHandler}
		go func() {
			fmt.Fprintf(os.Stderr, "admin web: http://%s\n", *webAddr)
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

	if a.Telegram == nil && !*enableCmd && *webAddr == "" {
		fmt.Fprintln(os.Stderr, "octosucker: need telegram bot_token, local cmd (-cmd=true), or -web address")
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
