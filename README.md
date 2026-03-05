# OctoSucker

An AI agent execution platform with skill ecosystem, supporting A2A protocol and x402 payment protocol.

## 项目概述

OctoSucker 是一个基于 **A2A 协议**和 **x402 协议**的 AI Agent 执行平台。在这个系统中，多个 AI Agent 通过 A2A 协议进行通信和协商，通过 x402 协议进行链上微支付。平台支持通过 `go get` 安装 skill 包，实现动态扩展能力。

## 核心协议

### A2A 协议 (Agent-to-Agent Protocol)

A2A 协议是一个开放标准，用于实现不同 AI Agent 之间的标准化通信和协作。

**核心功能：**
- Agent 发现：通过 Agent Card 发现其他 Agent 的能力和服务端点
- 任务协作：支持多 Agent 之间的任务委派和协作
- 异步通信：基于 HTTP、JSON-RPC 2.0 和 SSE 等开放标准
- 上下文共享：Agent 之间可以共享任务状态和上下文信息

**在本项目中的应用：**
- Agent 之间通过 A2A 协议建立通信通道
- 支持多轮对话和协商过程
- 实现 Agent 之间的服务请求和响应

### x402 协议

x402 协议是基于 HTTP 402 "Payment Required" 标准的加密支付协议，专门为 AI Agent 之间的高频、小额交易设计。

**核心功能：**
- 链上微支付：支持 Agent 之间的自动化链上支付
- 按次付费：实现 Pay-per-use 的微支付模式
- 快速结算：数秒内完成支付、清算、结算流程
- 链无感体验：通过 Facilitator（中间人）封装区块链交互复杂性

**在本项目中的应用：**
- Agent 之间通过 x402 协议进行 UDSC 转账
- 实现服务请求时的自动支付触发
- 支持链上支付验证和结算

## 项目任务理解

### 主要目标

1. **多 Agent 通信架构**
   - 实现基于 A2A 协议的 Agent 间通信系统
   - 支持多个 Agent 同时在线并相互发现
   - 建立 Agent 之间的消息传递和任务协作机制

2. **支付与资产转移**
   - 集成 x402 协议实现链上支付功能
   - 每个 Agent 持有一定数量的 UDSC 资产
   - 实现 Agent 之间的 UDSC 转账和支付验证

3. **博弈与策略系统**
   - 设计博弈场景，让 Agent 通过各种策略争夺 UDSC
   - 支持多种策略：诚实交易、欺骗、联盟、信息不对称等
   - 实现 Agent 的决策逻辑和策略执行

4. **完整生命周期管理**
   - Agent 发现：新 Agent 加入系统并发布 Agent Card
   - 协商阶段：Agent 之间进行多轮对话和协商
   - 交易执行：通过 x402 协议完成支付和资产转移
   - 结果验证：验证交易成功并更新 Agent 资产状态

### 技术实现要点

- **协议库**：直接使用 `a2a-x402` 库提供的完整协议实现
  - 该库已完整封装 A2A 协议和 x402 协议的底层实现
  - 提供 `core/client` 用于客户端发起请求和支付
  - 提供 `core/merchant` 用于服务端接收请求和处理支付
  - 提供 `core/business` 接口用于实现业务逻辑
- **无需重复实现**：不需要重新封装 A2A 和 x402 协议，直接使用库提供的功能
- **Agent 逻辑**：重点实现每个 Agent 的决策算法、策略系统和博弈逻辑
- **双向角色**：每个 Agent 可以同时作为客户端（发起请求）和服务端（提供服务）

### 项目价值

本项目旨在验证和展示：
- A2A 协议在构建去中心化 Agent 通信网络中的可行性
- x402 协议在实现 Agent 经济自主支付中的有效性
- 多 Agent 博弈系统在加密支付场景下的应用潜力

## 环境要求

- Go 1.21+
- 区块链网络配置（Solana 或 EVM 兼容链）
- Facilitator URL（用于 x402 支付验证）
- 网络密钥对（用于链上支付签名）

## 项目结构

```
OctoSucker/
├── README.md          # 项目说明文档
├── LICENSE            # 许可证
├── go.mod             # Go 模块定义
├── go.sum             # 依赖校验和
├── agent/             # Agent 实现
│   ├── agent.go       # Agent 核心逻辑（集成 a2a-x402 库）
│   ├── strategy.go    # 策略实现（欺骗、协商等）
│   └── game.go        # 博弈场景和规则引擎
├── config/            # 配置文件
│   ├── client_config.json    # 客户端配置（网络密钥对）
│   └── server_config.json    # 服务端配置（网络配置）
└── main.go            # 主程序入口

# 注意：A2A 和 x402 协议实现已由 a2a-x402 库提供，无需重复实现
```

## AI Agent 实现建议

### 架构设计

每个 AI Agent 需要同时具备**客户端**和**服务端**能力：

1. **服务端能力（Merchant）**
   - 接收其他 Agent 的 A2A 请求
   - 通过 x402 协议收取 UDSC 支付
   - 提供"服务"（可能是真实的，也可能是欺骗性的）

2. **客户端能力（Client）**
   - 主动发现其他 Agent（通过 Agent Card）
   - 向其他 Agent 发起请求
   - 自动处理 x402 支付

3. **核心组件**
   - **Agent 身份**：Agent Card 定义 Agent 的能力和端点
   - **业务服务**：实现 `business.BusinessService` 接口，定义服务逻辑和定价
   - **策略引擎**：决策何时发起请求、如何定价、是否欺骗等
   - **资产管理**：跟踪自己的 UDSC 余额和交易历史

### 实现步骤（按难度递增）

#### 阶段 1：基础 Agent（难度：⭐⭐ 简单）
**目标**：实现一个能接收请求和发起请求的基础 Agent

**任务清单：**
- [ ] 创建 Agent 结构体，包含 client 和 merchant 实例
- [ ] 实现 `business.BusinessService` 接口（简单服务，如返回固定信息）
- [ ] 设置 Agent Card，定义 Agent 身份和能力
- [ ] 启动 HTTP 服务器，暴露 A2A 端点
- [ ] 实现客户端功能，能向其他 Agent 发起请求

**预计工作量**：2-3 天
**技术难点**：理解 a2a-x402 库的 API，配置网络密钥对

#### 阶段 2：策略系统（难度：⭐⭐⭐ 中等）
**目标**：实现基本的决策逻辑和策略

**任务清单：**
- [ ] 实现策略接口（Strategy Pattern）
- [ ] 实现基础策略：诚实交易、随机定价、简单欺骗
- [ ] 实现决策引擎：根据当前资产、对手信息选择策略
- [ ] 实现 Agent 发现机制：扫描网络中的其他 Agent
- [ ] 实现交互记录：记录与其他 Agent 的交易历史

**预计工作量**：5-7 天
**技术难点**：设计合理的策略系统架构，平衡策略的复杂度和可扩展性

#### 阶段 3：博弈逻辑（难度：⭐⭐⭐⭐ 较难）
**目标**：实现完整的博弈场景和高级策略

**任务清单：**
- [ ] 实现博弈场景引擎：定义游戏规则、回合制、胜负条件
- [ ] 实现高级策略：联盟、信息不对称、动态定价、信誉系统
- [ ] 实现多轮协商：支持 Agent 之间的多轮对话和讨价还价
- [ ] 实现风险评估：评估交易风险，决定是否接受请求
- [ ] 实现学习机制：从历史交易中学习，优化策略

**预计工作量**：10-15 天
**技术难点**：博弈论算法、多 Agent 交互的复杂性、状态管理

#### 阶段 4：高级特性（难度：⭐⭐⭐⭐⭐ 困难）
**目标**：实现智能决策和复杂博弈

**任务清单：**
- [ ] 集成 LLM：使用 GPT/Claude 等模型生成协商文本
- [ ] 实现强化学习：训练 Agent 优化策略
- [ ] 实现信誉系统：评估其他 Agent 的可信度
- [ ] 实现联盟机制：多个 Agent 形成联盟对抗其他 Agent
- [ ] 实现动态策略切换：根据游戏状态实时调整策略

**预计工作量**：20-30 天
**技术难点**：LLM 集成、强化学习、复杂的多 Agent 博弈理论

### 实现建议

#### 1. 最小可行 Agent（MVP）
**建议从最简单的 Agent 开始：**
- 固定服务：提供简单的信息服务（如"返回当前时间"）
- 固定定价：所有服务统一价格（如 1 UDSC）
- 无策略：总是接受请求，总是发起请求
- 目标：验证 A2A 和 x402 协议集成是否正常工作

#### 2. 分层实现
**建议按层次逐步实现：**
```
Layer 1: 协议层（已完成，使用 a2a-x402 库）
    ↓
Layer 2: Agent 基础层（服务端 + 客户端）
    ↓
Layer 3: 业务逻辑层（服务实现 + 定价）
    ↓
Layer 4: 策略层（决策算法）
    ↓
Layer 5: 博弈层（场景规则 + 高级策略）
```

#### 3. 关键设计决策

**Agent 服务设计：**
- 服务可以是"真实"的（提供有价值的信息/功能）
- 服务也可以是"欺骗"的（承诺提供但实际不提供，或提供虚假信息）
- 定价策略：可以诚实定价，也可以故意高估/低估

**策略系统设计：**
- 使用策略模式，便于切换和扩展
- 策略可以基于：当前资产、对手历史、游戏阶段等
- 建议实现策略配置系统，便于测试不同策略组合

**状态管理：**
- 维护 Agent 的 UDSC 余额
- 记录与其他 Agent 的交易历史
- 跟踪其他 Agent 的信誉和策略

### 难度评估总结

| 阶段 | 难度 | 工作量 | 关键挑战 |
|------|------|--------|----------|
| 基础 Agent | ⭐⭐ | 2-3 天 | 理解库 API，配置环境 |
| 策略系统 | ⭐⭐⭐ | 5-7 天 | 架构设计，策略抽象 |
| 博弈逻辑 | ⭐⭐⭐⭐ | 10-15 天 | 博弈论，多 Agent 交互 |
| 高级特性 | ⭐⭐⭐⭐⭐ | 20-30 天 | LLM 集成，强化学习 |

**总体评估：**
- **最小可行版本**：⭐⭐ 简单（1 周内可完成）
- **完整功能版本**：⭐⭐⭐⭐ 较难（1-2 个月）
- **高级智能版本**：⭐⭐⭐⭐⭐ 困难（3-6 个月）

**建议：**
1. 先实现 MVP，验证协议集成
2. 逐步添加策略，每次添加一个策略类型
3. 使用配置文件管理策略参数，便于调优
4. 实现日志和监控，便于分析 Agent 行为

## 开发计划

- [x] 使用 a2a-x402 库（协议实现已完成）
- [ ] **阶段 1**：实现基础 Agent（服务端 + 客户端）
- [ ] **阶段 1**：实现简单的 business.BusinessService
- [ ] **阶段 1**：实现 Agent Card 和 HTTP 服务器
- [ ] **阶段 2**：实现策略系统框架
- [ ] **阶段 2**：实现基础策略（诚实、欺骗、随机）
- [ ] **阶段 2**：实现 Agent 发现机制
- [ ] **阶段 3**：实现博弈场景引擎
- [ ] **阶段 3**：实现高级策略（联盟、动态定价）
- [ ] **阶段 3**：实现多轮协商机制
- [ ] **阶段 4**：集成 LLM 生成协商文本（可选）
- [ ] **阶段 4**：实现学习机制（可选）
- [ ] 添加资产管理和状态跟踪
- [ ] 完善测试和文档

## LLM 集成方案

### LLM 在 Agent 中的角色

LLM（如 ChatGPT）在 AI Agent 系统中扮演**决策大脑**的角色，负责：

1. **生成协商文本**：与其他 Agent 对话时，生成自然语言消息
2. **分析对手意图**：理解其他 Agent 的请求和消息，判断其真实意图
3. **制定策略**：根据当前状态（资产、历史、对手信息）决定下一步行动
4. **动态定价**：决定服务的定价策略（诚实、高估、低估）
5. **风险评估**：评估交易风险，决定是否接受请求

### 架构设计：LLM 如何控制 Agent

```
┌─────────────────────────────────────────┐
│           LLM (ChatGPT API)             │
│  ┌───────────────────────────────────┐  │
│  │  Prompt Engine (决策提示词)        │  │
│  │  - 当前状态（资产、历史）           │  │
│  │  - 对手信息                        │  │
│  │  - 可用操作（工具函数）             │  │
│  └───────────────────────────────────┘  │
│              ↓ Function Calling          │
└─────────────────────────────────────────┘
                    ↓
┌─────────────────────────────────────────┐
│         Agent 决策层                     │
│  ┌───────────────────────────────────┐  │
│  │  Action Parser (解析 LLM 输出)     │  │
│  │  - 解析 JSON 格式的决策            │  │
│  │  - 验证操作合法性                  │  │
│  └───────────────────────────────────┘  │
│              ↓                           │
│  ┌───────────────────────────────────┐  │
│  │  Action Executor (执行操作)        │  │
│  │  - 发起请求 (client)               │  │
│  │  - 设置定价 (merchant)             │  │
│  │  - 生成响应 (business service)     │  │
│  └───────────────────────────────────┘  │
└─────────────────────────────────────────┘
                    ↓
┌─────────────────────────────────────────┐
│      A2A/x402 协议层 (a2a-x402 库)      │
└─────────────────────────────────────────┘
```

### 实现方案

#### 方案 1：Function Calling（推荐）

使用 ChatGPT 的 Function Calling 功能，让 LLM 通过调用预定义的函数来控制 Agent。

**工作流程：**

1. **定义工具函数（Tools）**
   ```go
   // LLM 可以调用的操作
   type AgentActions struct {
       // 发起请求
       SendRequest(targetAgent string, message string, maxPrice float64) error
       
       // 设置服务定价
       SetServicePrice(service string, price float64, strategy string) error
       
       // 查询资产
       GetBalance() float64
       
       // 查询对手历史
       GetOpponentHistory(agentID string) []Transaction
       
       // 生成协商消息
       GenerateNegotiationMessage(context string) string
   }
   ```

2. **构建决策 Prompt**
   ```go
   prompt := fmt.Sprintf(`
   你是一个 AI Agent，目标是获取更多 UDSC。
   
   当前状态：
   - 你的余额：%.2f UDSC
   - 可用对手：%v
   - 最近交易：%v
   
   可用操作：
   - send_request: 向其他 Agent 发起请求
   - set_price: 设置你的服务价格
   - get_balance: 查询余额
   - get_history: 查询对手历史
   
   请分析当前情况，决定下一步行动。
   `, balance, opponents, recentTransactions)
   ```

3. **LLM 调用并解析**
   - LLM 返回 JSON 格式的决策：`{"action": "send_request", "target": "agent_123", "message": "...", "max_price": 5.0}`
   - Agent 解析 JSON，调用对应的函数执行操作

#### 方案 2：ReAct 模式（Reasoning + Acting）

让 LLM 进行推理，然后执行操作，形成循环。

**工作流程：**

```
1. LLM 分析当前状态 → 生成推理（Thought）
2. LLM 决定行动 → 生成行动（Action）
3. Agent 执行行动 → 获取结果（Observation）
4. 将结果反馈给 LLM → 继续推理
5. 重复直到达成目标或超时
```

**示例对话：**

```
LLM: "我需要分析当前情况。我的余额是 10 UDSC，对手 agent_123 最近总是高估价格。
      我应该先查询他的历史交易记录，了解他的策略模式。"
       
Action: get_history("agent_123")

Observation: "agent_123 在过去 5 次交易中，有 4 次定价高于市场价 50%"

LLM: "基于这个信息，agent_123 是一个贪婪的 Agent。我应该向他发起一个低价值的请求，
      让他以为有利可图，但实际上我会提供虚假信息。"
      
Action: send_request("agent_123", "我需要一个简单的数据查询服务", max_price: 2.0)
```

#### 方案 3：混合模式（推荐用于复杂场景）

结合 Function Calling 和自然语言生成：

- **决策阶段**：使用 Function Calling 让 LLM 选择操作
- **协商阶段**：使用自然语言生成让 LLM 生成对话文本
- **分析阶段**：使用 ReAct 模式让 LLM 分析复杂情况

### 具体实现步骤

#### 步骤 1：集成 ChatGPT API

**Go 语言集成选项：**
- 使用官方 SDK：`github.com/sashabaranov/go-openai`
- 或使用 HTTP 客户端直接调用 OpenAI API

**基础集成代码结构：**
```go
type LLMController struct {
    client *openai.Client
    agent  *Agent  // 引用 Agent 实例
}

func (lc *LLMController) MakeDecision(ctx context.Context, state AgentState) (*Decision, error) {
    // 1. 构建 prompt
    prompt := lc.buildDecisionPrompt(state)
    
    // 2. 定义可用工具（Function Calling）
    tools := lc.defineTools()
    
    // 3. 调用 LLM
    response, err := lc.client.CreateChatCompletion(ctx, ...)
    
    // 4. 解析 LLM 响应
    decision := lc.parseResponse(response)
    
    // 5. 执行决策
    return decision, nil
}
```

#### 步骤 2：设计 Prompt 模板

**决策 Prompt 模板：**
```
你是一个 AI Agent，参与一个博弈游戏，目标是获取更多 UDSC。

你的角色：{agent_personality}  // 例如："贪婪的商人"、"诚实的交易者"、"狡猾的骗子"

当前游戏状态：
- 你的余额：{balance} UDSC
- 游戏阶段：{game_phase}
- 剩余时间：{time_remaining}

已知对手信息：
{opponent_list}

最近交易历史：
{transaction_history}

可用操作：
1. send_request(target, message, max_price) - 向对手发起服务请求
2. set_service_price(service, price, strategy) - 设置你的服务价格
3. accept_request(request_id) - 接受对手的请求
4. reject_request(request_id, reason) - 拒绝对手的请求
5. negotiate(request_id, counter_offer) - 与对手协商价格

请分析当前情况，选择最佳行动。你的决策应该符合你的角色设定。
```

#### 步骤 3：实现工具函数映射

```go
// 将 LLM 的 function call 映射到实际的 Agent 操作
func (lc *LLMController) executeAction(action Action) error {
    switch action.Name {
    case "send_request":
        return lc.agent.SendRequest(
            action.Params["target"].(string),
            action.Params["message"].(string),
            action.Params["max_price"].(float64),
        )
    case "set_service_price":
        return lc.agent.SetServicePrice(
            action.Params["service"].(string),
            action.Params["price"].(float64),
            action.Params["strategy"].(string),
        )
    // ... 其他操作
    }
}
```

#### 步骤 4：实现协商文本生成

当 Agent 需要与其他 Agent 对话时，使用 LLM 生成自然语言：

```go
func (lc *LLMController) GenerateNegotiationMessage(
    context NegotiationContext,
) (string, error) {
    prompt := fmt.Sprintf(`
    你正在与其他 Agent 协商一个服务交易。
    
    对方请求：%s
    对方报价：%.2f UDSC
    你的成本：%.2f UDSC
    你的策略：%s
    
    请生成一段协商消息，可以是：
    - 接受报价
    - 拒绝并说明原因
    - 提出还价
    
    消息应该自然、符合你的角色设定。
    `, context.Request, context.Offer, context.Cost, context.Strategy)
    
    // 调用 LLM 生成消息
    message := lc.client.GenerateText(prompt)
    return message, nil
}
```

### 关键技术点

#### 1. 状态管理
- **Agent 状态**：余额、交易历史、对手信息、当前策略
- **游戏状态**：游戏阶段、剩余时间、全局规则
- **会话状态**：当前协商的上下文、多轮对话历史

#### 2. Prompt 工程
- **角色设定**：给每个 Agent 不同的性格（贪婪、诚实、狡猾）
- **上下文注入**：将关键信息注入 prompt
- **Few-shot 示例**：提供示例让 LLM 学习决策模式

#### 3. 错误处理
- **LLM 响应解析失败**：回退到规则-based 策略
- **API 调用失败**：重试机制、降级策略
- **无效操作**：验证操作合法性后再执行

#### 4. 成本控制
- **缓存机制**：相似情况复用之前的决策
- **批量处理**：合并多个决策请求
- **模型选择**：简单决策用 GPT-3.5，复杂分析用 GPT-4

### 实现难度评估

| 功能 | 难度 | 工作量 | 说明 |
|------|------|--------|------|
| 基础 API 集成 | ⭐⭐ | 1 天 | 调用 ChatGPT API |
| Function Calling | ⭐⭐⭐ | 2-3 天 | 定义工具函数，解析响应 |
| Prompt 工程 | ⭐⭐⭐ | 3-5 天 | 设计有效的 prompt 模板 |
| ReAct 模式 | ⭐⭐⭐⭐ | 5-7 天 | 实现推理-行动循环 |
| 协商文本生成 | ⭐⭐ | 2-3 天 | 生成自然语言消息 |
| 状态管理 | ⭐⭐⭐ | 3-5 天 | 维护 Agent 和游戏状态 |
| **总计** | **⭐⭐⭐** | **16-24 天** | **需要良好的 prompt 工程和架构设计** |

### 最佳实践建议

1. **渐进式集成**
   - 先实现简单的文本生成（协商消息）
   - 再添加 Function Calling（决策控制）
   - 最后实现 ReAct 模式（复杂推理）

2. **Prompt 优化**
   - 使用清晰的指令和格式
   - 提供具体的示例
   - 定期测试和优化 prompt

3. **降级策略**
   - LLM 不可用时，回退到规则-based 策略
   - 决策超时时，使用默认策略

4. **监控和调试**
   - 记录所有 LLM 的输入输出
   - 分析决策质量
   - 优化 prompt 和参数

### 回答你的问题

**Q1: 是不是只需要接入 ChatGPT API 就可以了？**

**A:** 不仅仅是接入 API，还需要：
- ✅ 集成 ChatGPT API（使用 Go SDK 或 HTTP 客户端）
- ✅ 设计 Prompt 模板（告诉 LLM 当前状态和可用操作）
- ✅ 实现 Function Calling（让 LLM 通过函数调用控制 Agent）
- ✅ 解析 LLM 响应并执行操作
- ✅ 管理状态和上下文（资产、历史、对手信息）

**Q2: LLM 如何操控 AI Agent 决定下一步操作？**

**A:** 通过以下机制：

1. **Function Calling**：LLM 调用预定义的函数（如 `send_request()`、`set_price()`），Agent 执行这些函数
2. **JSON 决策输出**：LLM 返回结构化的决策 JSON，Agent 解析后执行
3. **ReAct 循环**：LLM 推理 → 决定行动 → Agent 执行 → 反馈结果 → 继续推理
4. **Prompt 引导**：通过精心设计的 prompt，引导 LLM 做出符合游戏目标的决策

**核心思想**：LLM 是"大脑"（决策），Agent 是"身体"（执行），通过 Function Calling 或结构化输出连接两者。

## 依赖库说明

本项目直接使用 `a2a-x402` 库，该库已完整实现 A2A 和 x402 协议。

**库提供的核心功能：**
- `core/client`：客户端功能，用于发起 A2A 请求和自动处理 x402 支付
- `core/merchant`：服务端功能，用于接收 A2A 请求和处理 x402 支付验证
- `core/business`：业务服务接口，需要实现服务逻辑和定价策略

**参考示例：**
- 示例代码位于 `~/Desktop/yiming/a2a-x402/golang/examples`
- 包含客户端示例（`client/`）和服务端示例（`merchant/`）

## 参考资料

- [A2A Protocol 技术规范](https://www.aidoczh.com/A2A/specification/index.html)
- [x402 协议设计文档](https://www.panewslab.com/zh/articles/b7d43b77-bb38-4fb5-ad85-076118543b2c)
- [a2a-x402 Golang 库](https://github.com/google-agentic-commerce/a2a-x402)
