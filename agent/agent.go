// Package agent 提供 AI Agent 的核心功能
//
// 重要原则：本项目不维护向后兼容性
// - 重构时直接删除旧代码，无需保留兼容层
// - 优化时优先考虑代码简洁性，而非向后兼容
// - 遇到"是否需要兼容"的问题时，答案永远是"不需要"
package agent

import (
	"context"
	"log"
	"sync"
	"time"

	skill "github.com/OctoSucker/octosucker-skill" // 导入时会自动执行 builtin_skill.go 的 init()，注册内置 Skill
	"github.com/OctoSucker/octosucker/agent/llm"
	"github.com/OctoSucker/octosucker/config"
)

type Agent struct {
	llmClient    *llm.LLMClient
	toolRegistry *skill.ToolRegistry

	// 任务队列系统（参考 OpenClaw）
	taskQueue  chan *Task          // 任务队列
	sessions   map[string]*Session // 会话管理（key: session ID）
	sessionsMu sync.RWMutex        // 会话锁

	// ReAct 循环配置
	maxReActIterations int           // 最大 ReAct 迭代次数（防止无限循环）
	taskTimeout        time.Duration // 单任务超时，0 表示不限制
	maxSessionAge      time.Duration // 会话最大存活时间

	// 配置管理
	configPath string // 配置文件路径（供 Tool 读取）
}

func NewAgent(
	ctx context.Context,
	configPath string,
) (*Agent, error) {

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	maxIter := 20
	taskTimeout := time.Duration(0)
	if cfg.Agent != nil {
		if cfg.Agent.MaxReActIterations > 0 {
			maxIter = cfg.Agent.MaxReActIterations
		}
		if cfg.Agent.TaskTimeoutSec > 0 {
			taskTimeout = time.Duration(cfg.Agent.TaskTimeoutSec) * time.Second
		}
	}

	agent := &Agent{
		llmClient:          llm.NewLLMClient(cfg.LLM.BaseURL, cfg.LLM.APIKey, cfg.LLM.Model),
		toolRegistry:       skill.NewToolRegistry(),
		taskQueue:          make(chan *Task, 100), // 任务队列，缓冲 100 个任务
		sessions:           make(map[string]*Session),
		maxReActIterations: maxIter,
		taskTimeout:        taskTimeout,
		maxSessionAge:      1 * time.Hour, // 会话最大存活 1 小时
		configPath:         configPath,
	}

	apiKeyPreview := cfg.LLM.APIKey
	if len(apiKeyPreview) > 10 {
		apiKeyPreview = apiKeyPreview[:10] + "..."
	}

	// 从配置文件构建 Skill 配置
	skillConfigs := buildSkillConfigsFromAgentConfig(cfg)

	// 加载通过 init() 注册的 Skill 包（包括内置 Skill），并由它们向 ToolRegistry 注册 Tool
	// 注意：即使某些 Skill Init 失败，也会继续加载其他 Skill，并记录失败信息
	failedSkills := skill.LoadAllRegisteredSkills(agent.toolRegistry, agent, skillConfigs)
	if len(failedSkills) > 0 {
		log.Printf("Warning: %d skill(s) failed to load:", len(failedSkills))
		for name, err := range failedSkills {
			log.Printf("  - %s: %v", name, err)
		}
		log.Printf("Note: Failed skills may still register their Tools, LLM can use list_skills Tool to check status")
	}

	skillNames := skill.GetRegisteredSkillNames()
	if len(skillNames) > 0 {
		log.Printf("Loaded %d skill(s) from Go packages", len(skillNames))
		for _, name := range skillNames {
			log.Printf("  - %s", name)
		}
	}

	return agent, nil
}

// Start 启动 Agent 的任务处理循环
func (a *Agent) Start(ctx context.Context) error {

	// 启动任务队列处理循环
	go a.runTaskQueueProcessor(ctx)

	// 启动会话清理循环（定期清理过期会话）
	go a.cleanupSessions(ctx)

	// 提交初始化任务
	a.submitInitializationTasks()

	// 等待上下文取消
	<-ctx.Done()
	log.Printf("Agent context cancelled, shutting down...")
	return ctx.Err()
}

// GetConfigPath 获取配置文件路径（供 Skill Tool 使用）
func (a *Agent) GetConfigPath() string {
	return a.configPath
}
