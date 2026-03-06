package main

import (
	"context"
	"flag"
	"log"

	"github.com/OctoSucker/octosucker/agent"

	_ "github.com/OctoSucker/skill-telegram"
)

// skill_imports.go 会自动生成在这里
// 这个文件由 Agent 在启动时自动生成，包含所有通过 go get 安装的 skill 包的导入

func init() {
	log.SetFlags(log.Ldate | log.Ltime | log.Llongfile)
}

func main() {
	configPath := flag.String("config", "config/agent_config.json", "Path to agent config file")
	testTask := flag.Bool("test-task", false, "Submit a test task to verify the agent is working")
	testInput := flag.String("test-input", "Hello, please list all available skills", "Input for the test task")
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

	// 如果指定了测试任务，提交一个测试任务
	if *testTask {
		log.Printf("Submitting test task with input: %s", *testInput)
		if err := agentInstance.SubmitTask(*testInput); err != nil {
			log.Printf("Failed to submit test task: %v", err)
		} else {
			log.Printf("Test task submitted successfully")
		}
	}

	if err := agentInstance.Start(ctx); err != nil {
		log.Fatalf("Failed to start agent: %v", err)
	}

}
