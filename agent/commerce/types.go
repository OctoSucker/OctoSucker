package commerce

// SimpleService 是一个简单的业务服务实现
// 这是阶段 1 的基础实现，后续可以扩展为更复杂的服务
type SimpleService struct {
	serviceName string
	basePrice   float64
}

// Transaction 表示一次交易
type Transaction struct {
	ID          string  `json:"id"`
	Type        string  `json:"type"`        // "sent" 或 "received"
	Target      string  `json:"target"`      // 对方 Agent URL
	Amount      float64 `json:"amount"`      // UDSC 金额
	Success     bool    `json:"success"`     // 是否成功
	Timestamp   int64   `json:"timestamp"`   // 时间戳
	Description string  `json:"description"` // 交易描述
}
