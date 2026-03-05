package commerce

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google-agentic-commerce/a2a-x402/core/business"
)

// NewSimpleService 创建一个简单的服务
func NewSimpleService(serviceName string, basePrice float64) *SimpleService {
	return &SimpleService{
		serviceName: serviceName,
		basePrice:   basePrice,
	}
}

// Execute 执行服务
// 当前实现：返回一个简单的 JSON 响应
func (s *SimpleService) Execute(ctx context.Context, prompt string) (string, error) {
	// 模拟服务执行（可以后续扩展为真实的服务逻辑）
	response := map[string]interface{}{
		"status":    "success",
		"service":   s.serviceName,
		"prompt":    prompt,
		"timestamp": time.Now().Unix(),
		"message":   fmt.Sprintf("Service '%s' executed successfully", s.serviceName),
	}

	jsonResponse, err := json.Marshal(response)
	if err != nil {
		return "", fmt.Errorf("failed to marshal response: %w", err)
	}

	return string(jsonResponse), nil
}

// ServiceRequirements 定义服务需求（价格、资源等）
func (s *SimpleService) ServiceRequirements(prompt string) business.ServiceRequirements {
	// 简单的定价策略：基础价格
	priceStr := fmt.Sprintf("%.2f", s.basePrice)

	description := fmt.Sprintf("Simple service: %s", s.serviceName)
	if len(prompt) > 50 {
		description = fmt.Sprintf("Simple service: %s - %s...", s.serviceName, prompt[:50])
	}

	return business.ServiceRequirements{
		Price:             priceStr,
		Resource:          fmt.Sprintf("/%s", s.serviceName),
		Description:       description,
		MimeType:          "application/json",
		Scheme:            "exact",
		MaxTimeoutSeconds: 300,
	}
}
