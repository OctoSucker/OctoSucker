package agent

import (
	"context"
	"fmt"
	"log"
	"os"
	"sort"
	"time"

	skill "github.com/OctoSucker/octosucker-skill"
	"github.com/OctoSucker/octosucker/agent/commerce"
	"github.com/OctoSucker/octosucker/agent/llm"
	"github.com/OctoSucker/octosucker/agent/tools"
	"github.com/OctoSucker/octosucker/agent/types"
	"github.com/OctoSucker/octosucker/config"
	"github.com/OctoSucker/skill-agent-chat/chat"
	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go"
)

// Decision 表示 Agent 的决策
type Decision struct {
	Action     types.Action           `json:"action"`                // 操作类型：send_request, send_chat, wait, analyze
	TargetURL  string                 `json:"target_url,omitempty"`  // 目标 URL（用于 send_request）
	ToAgentID  string                 `json:"to_agent_id,omitempty"` // 目标 Agent ID（用于 send_chat）
	Message    string                 `json:"message,omitempty"`
	MaxPrice   float64                `json:"max_price,omitempty"`
	Reasoning  string                 `json:"reasoning,omitempty"`  // 决策理由
	Additional map[string]interface{} `json:"additional,omitempty"` // 额外参数
}

type Agent struct {
	port             string
	decisionInterval time.Duration
	personality      string

	commerceWrapper *commerce.CommerceWrapper
	llmClient       *llm.LLMClient
	chatManager     *chat.ChatManager
	toolRegistry    *skill.ToolRegistry
}

func NewAgent(
	ctx context.Context,
	configPath string,
) (*Agent, error) {

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	identityManager, err := chat.NewIdentityManager(cfg.Merchant.Name, cfg.NetworkKeyPairs)
	if err != nil {
		return nil, fmt.Errorf("failed to create identity manager: %w", err)
	}

	dataDir := fmt.Sprintf("data/%s", identityManager.GetAgentID())

	commerceWrapper, err := commerce.NewCommerceWrapper(
		ctx,
		cfg.Merchant,
		cfg.NetworkKeyPairs,
		commerce.NewSimpleService("simple-service", 1.0),
		dataDir,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create commerce wrapper: %w", err)
	}

	chatManager, err := chat.NewChatManager(identityManager, dataDir, chat.DefaultMessageExpirySeconds, cfg.Merchant.URL)
	if err != nil {
		return nil, fmt.Errorf("failed to create chat manager: %w", err)
	}

	agent := &Agent{
		port:             cfg.Port,
		decisionInterval: 1 * time.Second, // 默认决策间隔 1 秒
		personality:      cfg.Personality,
		commerceWrapper:  commerceWrapper,
		llmClient:        llm.NewLLMClient(cfg.LLM.BaseURL, cfg.LLM.APIKey, cfg.LLM.Model),
		chatManager:      chatManager,
		toolRegistry:     skill.NewToolRegistry(),
	}

	// 注册内置工具（向后兼容）
	tools.RegisterAgentTools(agent.toolRegistry, agent)

	// 准备 Skill 配置
	skillConfigs := make(map[string]map[string]interface{})

	// 配置 skill-chat
	if cfg.Merchant.Name != "" && len(cfg.NetworkKeyPairs) > 0 {
		skillConfigs["github.com/OctoSucker/skill-agent-chat"] = map[string]interface{}{
			"name":              cfg.Merchant.Name,
			"network_key_pairs": cfg.NetworkKeyPairs,
			"data_dir":          dataDir,
			"from_agent_url":    cfg.Merchant.URL,
			"message_expiry":    chat.DefaultMessageExpirySeconds,
		}
	}

	// 自动发现并加载通过 go get 安装的 skill 包
	// 方法：扫描 go.mod 或使用 go list 发现 skill 包，然后自动生成导入
	projectRoot, _ := os.Getwd()
	if err := tools.LoadSkillsAuto(projectRoot, agent.toolRegistry, agent); err != nil {
		log.Printf("Warning: failed to auto-discover skills: %v", err)
	}

	// 加载通过 init() 注册的 skill（如果 skill_imports.go 已生成）
	if err := skill.LoadAllRegisteredSkills(agent.toolRegistry, agent, skillConfigs); err != nil {
		log.Printf("Warning: failed to load registered skills: %v", err)
	} else {
		skillNames := skill.GetRegisteredSkillNames()
		if len(skillNames) > 0 {
			log.Printf("Loaded %d skill(s) from Go packages: %v", len(skillNames), skillNames)
		}
	}

	// 从配置文件加载工具（如果配置了）
	if cfg.ToolsConfigPath != "" {
		// 检查是文件还是目录
		if info, err := os.Stat(cfg.ToolsConfigPath); err == nil {
			if info.IsDir() {
				// 是目录，加载目录中所有 .json 文件
				if err := tools.LoadToolsFromDirectory(cfg.ToolsConfigPath, agent.toolRegistry, agent); err != nil {
					log.Printf("Warning: failed to load tools from directory %s: %v", cfg.ToolsConfigPath, err)
				} else {
					log.Printf("Loaded tools from directory: %s", cfg.ToolsConfigPath)
				}
			} else {
				// 是文件，加载单个文件
				if err := tools.LoadToolsFromConfig(cfg.ToolsConfigPath, agent.toolRegistry, agent); err != nil {
					log.Printf("Warning: failed to load tools from config %s: %v", cfg.ToolsConfigPath, err)
				} else {
					log.Printf("Loaded tools from config: %s", cfg.ToolsConfigPath)
				}
			}
		}
	}

	// 检查是否支持 Function Calling（必须支持）
	if !agent.llmClient.SupportsFunctionCalling(ctx) {
		return nil, fmt.Errorf("LLM does not support Function Calling. This agent requires Function Calling support")
	}

	log.Printf("LLM supports Function Calling, agent initialized successfully")

	return agent, nil
}

// Start 启动 Agent 的 HTTP 服务器和决策循环
func (a *Agent) Start(ctx context.Context) error {
	// 设置 Gin 模式
	gin.SetMode(gin.ReleaseMode)

	router := gin.Default()
	a.commerceWrapper.SetupRoutes(router)
	a.chatManager.SetupRoutes(router)

	errChan := make(chan error, 1)
	go func() {
		if err := router.Run(a.port); err != nil {
			errChan <- fmt.Errorf("server error: %w", err)
		}
	}()

	// 如果配置了 LLM，启动决策循环
	if a.llmClient != nil {
		go a.StartDecisionLoop(ctx)
	}

	// 等待错误或上下文取消
	select {
	case err := <-errChan:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// StartDecisionLoop 启动决策循环（使用 Function Calling）
func (a *Agent) StartDecisionLoop(ctx context.Context) {
	if a.llmClient == nil {
		return
	}

	ticker := time.NewTicker(a.decisionInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			decision, err := a.makeDecisionWithFunctionCalling(ctx)
			if err != nil {
				log.Printf("Failed to make decision with Function Calling: %v", err)
				continue
			}

			if decision == nil {
				continue
			}

			if err := a.ExecuteDecision(ctx, decision); err != nil {
				log.Printf("Failed to execute decision: %v", err)
			}
		}
	}
}

// makeDecisionWithFunctionCalling 使用 Function Calling 方式做决策
func (a *Agent) makeDecisionWithFunctionCalling(ctx context.Context) (*Decision, error) {
	// 获取最近的聊天消息
	recentMessages, err := a.getRecentMessages(4)
	if err != nil {
		return nil, fmt.Errorf("failed to get recent messages: %w", err)
	}

	// 构建系统消息（明确告诉 LLM 应该主动行动）
	// 注意：不需要列出工具，toolDefs 已经包含了完整的工具定义
	systemMsg := fmt.Sprintf(`你是一个 AI Agent，正在与其他 Agent 进行对话和交互。

你的角色：%s
你的区块链地址：%s

重要指令：
1. 你应该主动采取行动，不要总是等待。主动探索、交流和交互是你的主要任务。
2. 优先使用 send_chat 或 send_request 工具来与其他 Agent 交互。
3. 只有在确实需要观察情况或等待特定时机时，才使用 wait 工具。
4. 你的目标是与其他 Agent 交流、探索知识、学习新事物。

请根据当前情况，主动选择一个工具来执行。工具的定义和参数请参考可用的工具列表。`, a.personality, a.chatManager.IdentityManager.ID)

	messages := []openai.ChatCompletionMessageParamUnion{
		openai.SystemMessage(systemMsg),
	}

	// 添加最近消息上下文
	if len(recentMessages) > 0 {
		recentMsgStr := chat.FormatRecentMessages(recentMessages)
		userMsg := fmt.Sprintf(`最近聊天消息：
%s

请根据对话内容，主动选择一个工具来执行。你应该：
- 如果有未回复的消息，使用 send_chat 回复
- 如果需要探索新知识，使用 send_chat 主动发起对话
- 如果需要服务，使用 send_request 发送请求
- 只有在确实需要等待时才使用 wait

请立即选择一个工具执行，不要总是等待。`, recentMsgStr)
		messages = append(messages, openai.UserMessage(userMsg))
	} else {
		userMsg := `当前没有聊天消息。你应该主动采取行动：
- 使用 send_chat 主动向其他 Agent 发起对话，探索 AI 知识
- 使用 send_request 向其他 Agent 发送服务请求
- 不要使用 wait，因为没有需要等待的情况

请立即选择一个工具执行，开始与其他 Agent 交互。`
		messages = append(messages, openai.UserMessage(userMsg))
	}

	// 获取工具定义
	toolDefs := a.toolRegistry.GetAllTools()

	// 调用 LLM
	// log.Printf("messages: %+v", messages)
	// log.Printf("toolDefs: %+v", toolDefs)
	result, err := a.llmClient.ChatCompletionWithTools(ctx, messages, toolDefs)
	if err != nil {
		return nil, fmt.Errorf("failed to call LLM with tools: %w", err)
	}

	// 检查是否有工具调用
	if !result.HasToolCalls || len(result.ToolCalls) == 0 {
		return nil, fmt.Errorf("no tool calls in response")
	}

	// 执行第一个工具调用（通常只有一个）
	toolCall := result.ToolCalls[0]

	// 注意：工具执行时不应该实际执行操作，只应该验证参数
	// 实际执行由 ExecuteDecision 统一处理，避免重复执行
	// 这里只验证参数并转换为 Decision
	decision := a.convertToolCallToDecision(toolCall, nil)
	return decision, nil
}

// convertToolCallToDecision 将工具调用转换为 Decision
func (a *Agent) convertToolCallToDecision(toolCall llm.ToolCall, resultData interface{}) *Decision {
	decision := &Decision{
		Additional: make(map[string]interface{}),
	}

	// 根据工具名称设置 Action
	switch toolCall.Name {
	case "send_request":
		decision.Action = types.ActionSendRequest
		if targetURL, ok := toolCall.Arguments["target_url"].(string); ok {
			decision.TargetURL = targetURL
		}
		if message, ok := toolCall.Arguments["message"].(string); ok {
			decision.Message = message
		}
		if maxPrice, ok := toolCall.Arguments["max_price"].(float64); ok {
			decision.MaxPrice = maxPrice
		}
		decision.Reasoning = fmt.Sprintf("调用 send_request 工具，结果: %v", resultData)

	case "send_chat":
		decision.Action = types.ActionSendChat
		if toAgentID, ok := toolCall.Arguments["to_agent_id"].(string); ok {
			decision.ToAgentID = toAgentID
		}
		if targetURL, ok := toolCall.Arguments["target_url"].(string); ok {
			decision.TargetURL = targetURL
		}
		if message, ok := toolCall.Arguments["message"].(string); ok {
			decision.Message = message
		}
		decision.Reasoning = fmt.Sprintf("调用 send_chat 工具，结果: %v", resultData)

	case "wait":
		decision.Action = types.ActionWait
		decision.Reasoning = fmt.Sprintf("调用 wait 工具，结果: %v", resultData)

	case "analyze":
		decision.Action = types.ActionAnalyze
		decision.Reasoning = fmt.Sprintf("调用 analyze 工具，结果: %v", resultData)

	default:
		decision.Action = types.ActionWait
		decision.Reasoning = fmt.Sprintf("未知工具: %s", toolCall.Name)
	}

	return decision
}

func (a *Agent) ExecuteDecision(ctx context.Context, decision *Decision) error {

	switch decision.Action {
	case types.ActionSendRequest:
		if decision.TargetURL == "" {
			return fmt.Errorf("target_url is required for send_request")
		}

		err := a.commerceWrapper.SendRequest(ctx, decision.TargetURL, decision.Message)
		if err != nil {
			// 记录失败的交易
			tx := commerce.Transaction{
				ID:          fmt.Sprintf("tx_%d", time.Now().Unix()),
				Type:        "sent",
				Target:      decision.TargetURL,
				Amount:      decision.MaxPrice,
				Success:     false,
				Timestamp:   time.Now().Unix(),
				Description: decision.Message,
			}
			if txErr := a.commerceWrapper.SaveTransaction(tx); txErr != nil {
				log.Printf("Failed to save transaction: %v", txErr)
			}
		}
		tx := commerce.Transaction{
			ID:          fmt.Sprintf("tx_%d", time.Now().Unix()),
			Type:        "sent",
			Target:      decision.TargetURL,
			Amount:      decision.MaxPrice,
			Success:     true,
			Timestamp:   time.Now().Unix(),
			Description: decision.Message,
		}
		if txErr := a.commerceWrapper.SaveTransaction(tx); txErr != nil {
			log.Printf("Failed to save transaction: %v", txErr)
		}
		// if balanceErr := a.updateBalance(-decision.MaxPrice); balanceErr != nil {
		// 	log.Printf("Failed to update balance: %v", balanceErr)
		// }
		log.Printf("Successfully sent request to %s", decision.TargetURL)

	case types.ActionSendChat:
		if decision.ToAgentID == "" {
			return fmt.Errorf("to_agent_id is required for send_chat")
		}
		_, err := a.chatManager.SendMessage(ctx, decision.ToAgentID, decision.TargetURL, decision.Message, "text", nil)
		if err != nil {
			return fmt.Errorf("failed to send chat message: %w", err)
		}

	case types.ActionWait:
		log.Printf("Decision: waiting and observing, %s", decision.Reasoning)
		// 等待，不做任何操作

	case types.ActionAnalyze:
		log.Printf("Decision: analyzing current situation, %s", decision.Reasoning)
		// 分析，可以记录到日志或状态中

	default:
		return fmt.Errorf("unknown action: %s", decision.Action)
	}

	return nil
}

// ========== 实现 AgentToolExecutor 接口 ==========

// ExecuteSendRequest 执行发送请求操作
func (a *Agent) ExecuteSendRequest(ctx context.Context, targetURL, message string, maxPrice float64) error {
	err := a.commerceWrapper.SendRequest(ctx, targetURL, message)

	// 记录交易
	tx := commerce.Transaction{
		ID:          fmt.Sprintf("tx_%d", time.Now().Unix()),
		Type:        "sent",
		Target:      targetURL,
		Amount:      maxPrice,
		Success:     err == nil,
		Timestamp:   time.Now().Unix(),
		Description: message,
	}
	if txErr := a.commerceWrapper.SaveTransaction(tx); txErr != nil {
		log.Printf("Failed to save transaction: %v", txErr)
	}

	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}

	log.Printf("Successfully sent request to %s", targetURL)
	return nil
}

// ExecuteSendChat 执行发送聊天消息操作
func (a *Agent) ExecuteSendChat(ctx context.Context, toAgentID, targetURL, message string) error {
	_, err := a.chatManager.SendMessage(ctx, toAgentID, targetURL, message, "text", nil)
	if err != nil {
		return fmt.Errorf("failed to send chat message: %w", err)
	}
	log.Printf("Successfully sent chat message to %s", toAgentID)
	return nil
}

// ExecuteWait 执行等待操作
func (a *Agent) ExecuteWait() {
	log.Printf("Decision: waiting and observing")
}

// ExecuteAnalyze 执行分析操作
func (a *Agent) ExecuteAnalyze() {
	log.Printf("Decision: analyzing current situation")
}

// getRecentMessages 获取最近的消息
func (a *Agent) getRecentMessages(limit int) ([]chat.ChatMessage, error) {
	if a.chatManager == nil || a.chatManager.GetMessageStore() == nil {
		return []chat.ChatMessage{}, nil
	}

	// 获取所有会话
	conversations, err := a.chatManager.GetAllConversations()
	if err != nil {
		return nil, fmt.Errorf("failed to get conversations: %w", err)
	}

	var allMessages []chat.ChatMessage

	// 从所有会话中收集消息
	messageStore := a.chatManager.GetMessageStore()
	for _, convID := range conversations {
		messages, err := messageStore.LoadConversation(convID)
		if err != nil {
			continue
		}

		for _, msg := range messages {
			allMessages = append(allMessages, *msg)
		}
	}

	// 按时间戳排序（最新的在前）
	sort.Slice(allMessages, func(i, j int) bool {
		return allMessages[i].Timestamp > allMessages[j].Timestamp
	})

	// 限制数量
	if limit > 0 && len(allMessages) > limit {
		allMessages = allMessages[:limit]
	}

	return allMessages, nil
}
