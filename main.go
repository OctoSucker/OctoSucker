package main

import (
	"context"
	"flag"
	"log"

	"github.com/OctoSucker/octosucker/agent"
	"github.com/OctoSucker/octosucker/config"

	_ "github.com/OctoSucker/tools-cron"
	_ "github.com/OctoSucker/tools-exec"
	_ "github.com/OctoSucker/tools-fs"
	_ "github.com/OctoSucker/tools-mcp"
	_ "github.com/OctoSucker/tools-remember"
	_ "github.com/OctoSucker/tools-telegram"
	_ "github.com/OctoSucker/tools-web"
)

func init() {
	log.SetFlags(log.Ldate | log.Ltime | log.Llongfile)
}

func main() {
	configPath := flag.String("config", "config/agent_config.json", "Path to agent config file")
	flag.Parse()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	llmCfg, reactCfg, toolProviderConfigs, err := config.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	agentInstance, err := agent.NewAgent(
		ctx,
		*configPath,
		llmCfg,
		reactCfg,
		toolProviderConfigs,
		config.LoadSystemPrompt().SystemPrompt,
	)
	if err != nil {
		log.Fatalf("Failed to create agent: %v", err)
	}

	if err := agentInstance.Start(ctx, config.LoadSystemPrompt().StartupTasks); err != nil {
		log.Fatalf("Failed to start agent: %v", err)
	}
}
