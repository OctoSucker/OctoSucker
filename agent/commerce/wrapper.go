package commerce

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/OctoSucker/octosucker/config"
	"github.com/a2aproject/a2a-go/a2a"
	"github.com/a2aproject/a2a-go/a2asrv"
	"github.com/gin-gonic/gin"
	"github.com/google-agentic-commerce/a2a-x402/core/business"
	"github.com/google-agentic-commerce/a2a-x402/core/client"
	"github.com/google-agentic-commerce/a2a-x402/core/merchant"
	"github.com/google-agentic-commerce/a2a-x402/core/types"
	"github.com/google-agentic-commerce/a2a-x402/core/x402"
)

// CommerceWrapper 统一的 Commerce 包装器，整合 Merchant、Client 和 TransactionStore 功能
type CommerceWrapper struct {
	// Merchant 相关
	agentCard *a2a.AgentCard
	handler   a2asrv.RequestHandler

	// Client 相关
	networkKeyPairs []types.NetworkKeyPair

	// TransactionStore 相关
	dataDir               string
	transactionsFile      string
	maxRecentTransactions int
}

// NewCommerceWrapper 创建一个新的 Commerce 包装器
func NewCommerceWrapper(
	ctx context.Context,
	merchantConfig *config.Merchant,
	networkKeyPairs []types.NetworkKeyPair,
	businessService business.BusinessService,
	dataDir string,
) (*CommerceWrapper, error) {
	// 创建 Merchant 实例
	merchantInstance, err := merchant.NewMerchant(ctx, merchantConfig.FacilitatorURL, businessService, merchantConfig.NetworkConfigs)
	if err != nil {
		return nil, fmt.Errorf("failed to create merchant: %w", err)
	}

	// 创建 Agent Card
	agentCard := &a2a.AgentCard{
		Name:               merchantConfig.Name,
		Description:        merchantConfig.Description,
		URL:                fmt.Sprintf("%s/rpc", merchantConfig.URL),
		PreferredTransport: a2a.TransportProtocolJSONRPC,
		DefaultInputModes:  []string{"text"},
		DefaultOutputModes: []string{"text"},
		Capabilities: a2a.AgentCapabilities{
			Extensions: []a2a.AgentExtension{
				{
					URI:      x402.X402ExtensionURI,
					Required: true,
				},
			},
		},
		ProtocolVersion: "0.2",
		Version:         "1.0.0",
		Skills: []a2a.AgentSkill{
			{
				Name:        "simple-service",
				Description: "A simple service provided by this agent",
			},
		},
	}

	// 初始化数据目录
	if dataDir == "" {
		dataDir = "data"
	}
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create data directory: %w", err)
	}

	return &CommerceWrapper{
		agentCard:             agentCard,
		handler:               a2asrv.NewHandler(merchantInstance.Orchestrator()),
		networkKeyPairs:       networkKeyPairs,
		dataDir:               dataDir,
		transactionsFile:      filepath.Join(dataDir, "recent_transactions.json"),
		maxRecentTransactions: 20, // 默认保留最近 20 条交易
	}, nil
}

// SetupRoutes 设置 Commerce 相关的路由（Merchant 功能）
func (cw *CommerceWrapper) SetupRoutes(router *gin.Engine) {
	agentCardHandler := a2asrv.NewStaticAgentCardHandler(cw.agentCard)
	router.GET(a2asrv.WellKnownAgentCardPath, gin.WrapH(agentCardHandler))

	rpcHandler := a2asrv.NewJSONRPCHandler(cw.handler)
	wrappedHandler := extractHeadersMiddleware(rpcHandler)
	router.POST("/rpc", gin.WrapH(wrappedHandler))
	router.GET("/rpc", gin.WrapH(wrappedHandler))
}

// SendRequest 向其他 Agent 发送请求（Client 功能）
func (cw *CommerceWrapper) SendRequest(ctx context.Context, targetURL, message string) error {
	// 创建新的 client 实例，指向目标 URL
	clientInstance, err := client.NewClient(targetURL, cw.networkKeyPairs)
	if err != nil {
		return fmt.Errorf("failed to create client for target: %w", err)
	}

	// 发送请求并等待完成
	_, err = clientInstance.WaitForCompletion(ctx, message)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}

	return nil
}

// LoadTransactions 从 JSON 文件加载交易记录（TransactionStore 功能）
func (cw *CommerceWrapper) LoadTransactions() ([]Transaction, error) {
	data, err := os.ReadFile(cw.transactionsFile)
	if err != nil {
		if os.IsNotExist(err) {
			return []Transaction{}, nil // 文件不存在，返回空列表
		}
		return nil, fmt.Errorf("failed to read transactions file: %w", err)
	}

	var transactions []Transaction
	if err := json.Unmarshal(data, &transactions); err != nil {
		return nil, fmt.Errorf("failed to unmarshal transactions: %w", err)
	}

	return transactions, nil
}

// SaveTransaction 保存交易记录到 JSON 文件（TransactionStore 功能）
func (cw *CommerceWrapper) SaveTransaction(tx Transaction) error {
	// 加载现有交易记录
	transactions, err := cw.LoadTransactions()
	if err != nil {
		transactions = []Transaction{} // 如果加载失败，使用空列表
	}

	// 添加新交易
	transactions = append(transactions, tx)

	// 只保留最近 N 条交易
	if len(transactions) > cw.maxRecentTransactions {
		transactions = transactions[len(transactions)-cw.maxRecentTransactions:]
	}

	// 保存到文件
	return cw.saveTransactions(transactions)
}

// saveTransactions 保存交易记录列表到文件（TransactionStore 功能）
func (cw *CommerceWrapper) saveTransactions(transactions []Transaction) error {
	if err := os.MkdirAll(cw.dataDir, 0755); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	data, err := json.MarshalIndent(transactions, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal transactions: %w", err)
	}

	// 使用临时文件 + 原子重命名，确保写入安全
	tmpFile := cw.transactionsFile + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write transactions file: %w", err)
	}

	if err := os.Rename(tmpFile, cw.transactionsFile); err != nil {
		return fmt.Errorf("failed to rename transactions file: %w", err)
	}

	return nil
}

// GetAllTransactions 获取所有交易记录（TransactionStore 功能）
func (cw *CommerceWrapper) GetAllTransactions() ([]Transaction, error) {
	return cw.LoadTransactions()
}

// extractHeadersMiddleware 提取请求头并添加到上下文
func extractHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers := make(map[string][]string)
		for k, v := range r.Header {
			headers[k] = v
		}

		requestMeta := a2asrv.NewRequestMeta(headers)
		ctx, _ := a2asrv.WithCallContext(r.Context(), requestMeta)
		r = r.WithContext(ctx)

		next.ServeHTTP(w, r)
	})
}
