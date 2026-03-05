package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	skill "github.com/OctoSucker/octosucker-skill"
)

// ToolConfig 工具配置（从文件加载）
type ToolConfig struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
	HandlerType string                 `json:"handler_type"`           // 内置 Handler 类型，如 "send_chat", "send_request", "custom"
	HandlerCode string                 `json:"handler_code,omitempty"` // 自定义 Handler 代码（未来支持）
}

// LoadToolsFromConfig 从配置文件加载工具
func LoadToolsFromConfig(configPath string, registry *skill.ToolRegistry, agentInstance AgentToolExecutor) error {
	// 检查配置文件是否存在
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		// 配置文件不存在，只使用内置工具
		return nil
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to read tools config: %w", err)
	}

	var toolConfigs []ToolConfig
	if err := json.Unmarshal(data, &toolConfigs); err != nil {
		return fmt.Errorf("failed to parse tools config: %w", err)
	}

	// 注册每个工具
	for _, toolConfig := range toolConfigs {
		tool := &skill.Tool{
			Name:        toolConfig.Name,
			Description: toolConfig.Description,
			Parameters:  toolConfig.Parameters,
			Handler:     createHandlerFromConfig(toolConfig, agentInstance),
		}

		if tool.Handler == nil {
			return fmt.Errorf("tool %s: handler_type %s is not supported or invalid", toolConfig.Name, toolConfig.HandlerType)
		}

		registry.Register(tool)
	}

	return nil
}

// createHandlerFromConfig 根据配置创建 Handler
func createHandlerFromConfig(config ToolConfig, agentInstance AgentToolExecutor) skill.ToolHandler {
	switch config.HandlerType {
	case "send_chat":
		return func(params map[string]interface{}) (interface{}, error) {
			toAgentID, _ := params["to_agent_id"].(string)
			targetURL, _ := params["target_url"].(string)
			message, _ := params["message"].(string)

			if toAgentID == "" {
				return nil, fmt.Errorf("to_agent_id is required")
			}
			if message == "" {
				return nil, fmt.Errorf("message is required")
			}

			// 只验证参数，不实际执行
			return map[string]interface{}{
				"success":     true,
				"to_agent_id": toAgentID,
				"target_url":  targetURL,
				"message":     message,
				"validated":   true,
			}, nil
		}

	case "send_request":
		return func(params map[string]interface{}) (interface{}, error) {
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

			// 只验证参数，不实际执行
			return map[string]interface{}{
				"success":    true,
				"target_url": targetURL,
				"message":    message,
				"max_price":  maxPrice,
				"validated":  true,
			}, nil
		}

	case "wait":
		return func(params map[string]interface{}) (interface{}, error) {
			agentInstance.ExecuteWait()
			return map[string]interface{}{
				"success": true,
				"action":  "waiting",
			}, nil
		}

	case "analyze":
		return func(params map[string]interface{}) (interface{}, error) {
			agentInstance.ExecuteAnalyze()
			return map[string]interface{}{
				"success": true,
				"action":  "analyzing",
			}, nil
		}

	case "custom":
		// 未来支持自定义脚本执行
		return func(params map[string]interface{}) (interface{}, error) {
			return map[string]interface{}{
				"success": false,
				"error":   "custom handler not yet implemented",
			}, nil
		}

	default:
		return nil
	}
}

// LoadToolsFromDirectory 从目录加载所有工具配置文件
func LoadToolsFromDirectory(dirPath string, registry *skill.ToolRegistry, agentInstance AgentToolExecutor) error {
	// 检查目录是否存在
	if _, err := os.Stat(dirPath); os.IsNotExist(err) {
		// 目录不存在，只使用内置工具
		return nil
	}

	// 读取目录中的所有 .json 文件
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return fmt.Errorf("failed to read tools directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		// 只处理 .json 文件
		if filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		configPath := filepath.Join(dirPath, entry.Name())
		if err := LoadToolsFromConfig(configPath, registry, agentInstance); err != nil {
			// 记录错误但继续加载其他文件
			fmt.Printf("Warning: failed to load tools from %s: %v\n", configPath, err)
			continue
		}
	}

	return nil
}
