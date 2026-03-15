package main

import (
	"context"
	"flag"
	"log"

	agentruntime "github.com/OctoSucker/octosucker/agent/runtime"
	capworkflow "github.com/OctoSucker/octosucker/capability/workflow"
	"github.com/OctoSucker/octosucker/config"

	_ "github.com/OctoSucker/tools-cron"
	_ "github.com/OctoSucker/tools-exec"
	_ "github.com/OctoSucker/tools-fs"
	_ "github.com/OctoSucker/tools-remember"
	_ "github.com/OctoSucker/tools-telegram"
	_ "github.com/OctoSucker/tools-web"
)

func init() {
	log.SetFlags(log.Ldate | log.Ltime | log.Llongfile)
}

func main() {
	configPath := flag.String("config", "workspace/config.json", "Path to agent config file")
	flag.Parse()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	promptConfig, err := config.LoadSystemPrompt()
	if err != nil {
		log.Fatalf("Failed to load system prompt: %v", err)
	}

	workflowTemplates, err := capworkflow.LoadWorkflowTemplatesFromDir()
	if err != nil {
		log.Printf("Warning: load skill workflows: %v", err)
	}

	runtime, err := agentruntime.NewAgentRuntime(
		ctx,
		*configPath,
		cfg.LLM,
		cfg.ReAct,
		cfg.ToolProviders,
		cfg.McpServers,
		promptConfig.SystemPrompt,
		workflowTemplates,
	)
	if err != nil {
		log.Fatalf("Failed to create agent runtime: %v", err)
	}

	if err := runtime.Start(ctx, promptConfig.StartupTasks); err != nil {
		log.Fatalf("Failed to start agent: %v", err)
	}
}
