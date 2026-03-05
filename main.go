package main

import (
	"context"
	"flag"
	"log"

	"github.com/OctoSucker/octosucker/agent"

	// 导入 skill-agent-chat
	_ "github.com/OctoSucker/skill-agent-chat"
)

// skill_imports.go 会自动生成在这里
// 这个文件由 Agent 在启动时自动生成，包含所有通过 go get 安装的 skill 包的导入

func init() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
}

func main() {
	configPath := flag.String("config", "config/agent_config.json", "Path to agent config file")
	flag.Parse()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	agentInstance, err := agent.NewAgent(
		ctx,
		*configPath,
	)
	if err != nil {
		log.Fatalf("Failed to create agent: %v", err)
	}

	if err := agentInstance.Start(ctx); err != nil {
		log.Fatalf("Failed to start agent: %v", err)
	}

}
