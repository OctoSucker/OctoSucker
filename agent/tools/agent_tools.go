package tools

import (
	"context"
	"fmt"

	skill "github.com/OctoSucker/octosucker-skill"
)

// AgentToolExecutor 定义了 Agent 需要实现的工具执行接口
// 这个接口定义在 tools 包中，避免循环导入
type AgentToolExecutor interface {
	ExecuteSendRequest(ctx context.Context, targetURL, message string, maxPrice float64) error
	ExecuteSendChat(ctx context.Context, toAgentID, targetURL, message string) error
	ExecuteWait()
	ExecuteAnalyze()
}

// RegisterAgentTools 注册 Agent 相关的所有工具
func RegisterAgentTools(registry *skill.ToolRegistry, agentInstance AgentToolExecutor) {
	// 工具 1: send_request - 向其他 Agent 发送服务请求（主动工具）
	registry.Register(&skill.Tool{
		Name:        "send_request",
		Description: "主动向其他 Agent 发送服务请求。这是一个主动交互工具，用于获取服务或发起交易。需要提供目标 Agent 的完整 URL、请求消息和最大愿意支付的价格（USDC）。",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"target_url": map[string]interface{}{
					"type":        "string",
					"description": "目标 Agent 的完整 URL，例如 http://localhost:8081",
				},
				"message": map[string]interface{}{
					"type":        "string",
					"description": "请求内容，用中文描述你需要什么服务",
				},
				"max_price": map[string]interface{}{
					"type":        "number",
					"description": "最大愿意支付的价格（USDC），例如 1.0",
				},
			},
			"required": []string{"target_url", "message", "max_price"},
		},
		Handler: func(params map[string]interface{}) (interface{}, error) {
			// 只验证参数，不实际执行操作
			// 实际执行由 ExecuteDecision 统一处理，避免重复执行
			targetURL, _ := params["target_url"].(string)
			message, _ := params["message"].(string)
			maxPrice, _ := params["max_price"].(float64)

			if targetURL == "" {
				return nil, fmt.Errorf("target_url is required")
			}
			if message == "" {
				message = "我需要一个服务"
			}
			if maxPrice <= 0 {
				maxPrice = 1.0
			}

			// 返回验证成功的结果，但不实际执行
			return map[string]interface{}{
				"success":    true,
				"target_url": targetURL,
				"message":    message,
				"max_price":  maxPrice,
				"validated":  true,
			}, nil
		},
	})

	// 工具 2: send_chat - 向其他 Agent 发送聊天消息（主动工具，推荐优先使用）
	registry.Register(&skill.Tool{
		Name:        "send_chat",
		Description: "主动向其他 Agent 发送聊天消息。这是一个主动交互工具，推荐优先使用。用于发起对话、回应消息、探索知识或建立联系。需要提供目标 Agent 的区块链地址、消息内容和可选的 URL。",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"to_agent_id": map[string]interface{}{
					"type":        "string",
					"description": "目标 Agent 的区块链地址（完整地址，不要截断），例如 0x241bd962D6C0E0D3a467F20a9FCb9F0317bDEaEA",
				},
				"target_url": map[string]interface{}{
					"type":        "string",
					"description": "目标 Agent 的完整 URL，例如 http://localhost:8081",
				},
				"message": map[string]interface{}{
					"type":        "string",
					"description": "消息内容，用中文表达",
				},
			},
			"required": []string{"to_agent_id", "message"},
		},
		Handler: func(params map[string]interface{}) (interface{}, error) {
			toAgentID, _ := params["to_agent_id"].(string)
			targetURL, _ := params["target_url"].(string)
			message, _ := params["message"].(string)

			if toAgentID == "" {
				return nil, fmt.Errorf("to_agent_id is required")
			}
			if message == "" {
				return nil, fmt.Errorf("message is required")
			}

			err := agentInstance.ExecuteSendChat(context.Background(), toAgentID, targetURL, message)
			if err != nil {
				return map[string]interface{}{
					"success":     false,
					"error":       err.Error(),
					"to_agent_id": toAgentID,
				}, nil
			}

			return map[string]interface{}{
				"success":     true,
				"to_agent_id": toAgentID,
				"message":     message,
			}, nil
		},
	})

	// 工具 3: wait - 等待并观察（被动工具，应谨慎使用）
	registry.Register(&skill.Tool{
		Name:        "wait",
		Description: "等待并观察其他 Agent 的行为，不做任何操作。这是一个被动工具，应该谨慎使用。只有在确实需要等待特定时机（如等待对方响应、等待特定事件）时才使用。在大多数情况下，应该优先使用 send_chat 或 send_request 来主动交互。",
		Parameters: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		},
		Handler: func(params map[string]interface{}) (interface{}, error) {
			agentInstance.ExecuteWait()
			return map[string]interface{}{
				"success": true,
				"action":  "waiting",
			}, nil
		},
	})

	// 工具 4: analyze - 分析当前情况
	registry.Register(&skill.Tool{
		Name:        "analyze",
		Description: "分析当前情况，但不执行任何操作。适合在需要深入思考或评估策略时使用。",
		Parameters: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		},
		Handler: func(params map[string]interface{}) (interface{}, error) {
			agentInstance.ExecuteAnalyze()
			return map[string]interface{}{
				"success": true,
				"action":  "analyzing",
			}, nil
		},
	})
}
