# OctoSucker

OctoSucker 是一个用于探索实用型个人 AI 助理应当如何设计的学习项目。它并不试图复刻代码 Agent 的全部功能，而是聚焦一个更具体的问题：当 Agent 真正操作工具、完成工作时，如何让它既容易理解和扩展，又能被普通桌面用户自然使用？

目前的答案是一个使用 Go 编写的串行运行时。它通过异步 Task API 对外提供服务，并刻意保持推理循环短小、清晰、可检查：

```text
Planner -> Executor -> Step Evaluator -> Planner or Responder
```

Planner 每次只选择一个动作。每次工具调用、信息补充请求和模型直接回复都会形成用户可见的 Step。运行时可以暂停并等待结构化输入或风险确认，之后继续执行同一个 Task。

## 设计原则

### 一个助理，多个 Task

用户面对的应该是一个持续存在的数字助理，而不是一组相互割裂的聊天会话。但在系统内部，工作仍然需要划分为多个 Task，因为每次执行都需要明确的生命周期、失败状态、审批边界和审计记录。因此，Task 是执行单元，而不是产品要求用户理解的主要概念。

当 Task 缺少信息时，用户的下一次输入会继续当前 Task。Task 完成后，新的请求会创建一个子 Task，同时保留助理上下文。这样既能维持连续的使用体验，也不会把互不相关的工作伪装成同一次执行。

### 根据当前证据，每次只规划一步

OctoSucker 不要求模型在执行开始前编造一张完整的执行图。对于开放式办公任务，后续动作往往取决于工具结果，或者取决于用户尚未提供的信息。一份庞大的预先计划可能看起来很完整，实际上却建立在未经验证的假设上。

Planner 会根据当前目标、上下文和已有观察选择下一个动作。动作执行后，Evaluator 判断结果是否推动了目标，再由 Planner 继续决策：

```text
目标 + 上下文
      |
      v
规划一个动作 -> 执行 -> 评估有效性与目标进度
      ^                         |
      |_________________________|
                  |
                  v
          回复用户或请求补充信息
```

当前串行执行是有意为之。并行可以提高吞吐量，但也会增加协调成本，让早期行为更难理解。对于仍在验证中的个人助理，可调试性和可预测的状态转换比过早引入并发更重要。

### 区分执行事实与目标完成

工具调用成功，只能证明工具完成了自己的契约，并不能证明用户目标已经实现。OctoSucker 将内置工具、MCP 服务或类型化 Provider 返回的结构化结果视为执行事实，再由 Evaluator 根据当前 Step 目标和用户总目标判断结果是否有效。

这种设计避免两个相反的错误：一是看到命令成功退出就直接宣布任务完成；二是不相信任何工具返回值，不断追加核验，最终陷入无穷的不信任链。工具正确性应由工具边界负责，目标层面的判断应由 Agent Loop 负责。

### 让执行过程可观察，而不是暴露隐藏思维链

界面应该告诉用户 Agent 正在做什么，但不应展示模型的私有推理。每个工具动作、信息请求和直接回复都会形成一个简洁的 Step，其中包含标题、状态、摘要和评估。失败尝试也会保留在执行轨迹中，使后续决策和问题排查基于真实发生过的过程。

用户看到的是执行进度和决策结果，而不是原始 Prompt 或隐藏推理文本。

### 信息是结构化的，交互也应当结构化

一个空白对话框会把描述任务的全部负担交给用户。对于已经理解 Prompt、工具和文件路径的人，这种方式很灵活；但对于普通办公用户，它也构成了明显的使用门槛。

自然语言仍然适合表达开放意图。当 Agent 已经知道缺少哪些具体字段时，它可以返回带有标签、选项、校验和默认值的结构化表单。高风险动作同样应在正常对话流程中转换成明确的审批控件。界面需要适应当前 Task，而不是强迫所有交互都通过文字描述完成。

### 保持运行时通用，让能力边界具备确定性

核心循环不应包含针对某个网站、市场、消息产品或办公流程的特殊逻辑。新能力通过明确的边界接入：

- MCP 用于结构化的外部服务。
- 类型化可执行 Provider 用于确定性地构造命令。
- Agent Skill 用于提供过程知识和工作流指导。

Skill 文档可以告诉模型何时、为何使用某项能力，但不应迫使模型从自然语言中重建可执行协议。稳定的命令名称、参数、校验规则和返回结构应当放在 MCP 或类型化 Provider 中。

### 保守地学习

历史执行可以改善未来的工具路由，但学习结果只能作为建议。它可以帮助 Planner 在多个能力之间做选择，却不能绕过规划、Schema 校验、风险策略或结果评估。这样既能利用经验，也不会让历史噪声变成隐藏的控制逻辑。

### 明确产品边界，避免过早建设基础设施

OctoSucker 当前选择本地运行、用户自有、串行执行、低频 HTTP 轮询和内存 Task 状态。这些都是针对单用户学习项目的明确取舍。Task 持久化、并行执行、子 Agent 和流式传输，应当在真实使用证明其价值后再加入，而不是因为成熟 Agent 产品拥有这些功能就提前照搬。

## 当前能力

- 串行、单步规划，并具有重试次数和执行上限。
- Planner、Evaluator 和 Responder 可分别配置不同模型。
- 按角色分配上下文预算，并对执行轨迹进行结构化压缩。
- 内置工具、MCP 和类型化可执行工具统一进入经过 Schema 校验的工具目录。
- 目录式 Agent Skill，支持显式激活和按需读取资源。
- 异步 Task，包含消息、Step、状态、结果和错误信息。
- Agent 缺少信息时可生成结构化表单。
- 高风险工具执行前需要用户明确批准。
- 学习到的工具路由只能提供建议，不能绕过 Planner。
- 支持本地 HTTP 入口和可选的 Telegram 入口。

Task 状态目前只保存在内存中。进程重启后 Task 会被清除，但 Workspace SQLite 数据、路由学习数据、Skill、知识文件和日志仍保存在磁盘中。

## 代码架构

| 路径 | 职责 |
| --- | --- |
| `cmd/octosucker` | 服务入口和生命周期管理 |
| `config` | Workspace 配置和路径解析 |
| `internal/runtime/agentloop` | 串行 Turn 控制和执行限制 |
| `internal/runtime/model` | 只追加的 Turn、Step、Observation 和 Assessment 状态 |
| `internal/runtime/contextmanager` | 按角色选择和压缩上下文 |
| `internal/runtime/planning` | 生成结构化的执行或回复决策 |
| `internal/runtime/execution` | 工具调用、风险审批和结果标准化 |
| `internal/runtime/evaluation` | 评估 Step 有效性和目标进度 |
| `internal/runtime/responding` | 生成面向用户的最终回复 |
| `internal/runtime/conversation` | 维护有界的助理上下文 |
| `internal/runtime/toolrouting` | 提供保守的学习型路由建议 |
| `internal/task` | 内存 Task 生命周期和快照 |
| `internal/interaction` | 结构化用户输入表单 |
| `internal/skills` | Skill 发现、校验、激活和资源读取 |
| `internal/toolcontract` | 工具 DTO、策略和 JSON Schema 校验 |
| `internal/tools` | 内置工具、MCP、OpenCLI 和可执行 Provider |
| `internal/storage` | Workspace SQLite 持久化 |
| `internal/gateway` | 组装 HTTP 和 Telegram 入口 |
| `internal/ingress/adminhttp` | Task、兼容 Chat 和 Graph HTTP 路由 |

运行时契约和循环不变量见 [`internal/runtime/DESIGN.md`](internal/runtime/DESIGN.md)。

## Workspace

服务需要一个包含 `config.json` 和 Agent 自有资源的 Workspace 目录：

```text
workspace/
|-- config.json
|-- skills/
|-- knowledge/
|-- data/
`-- logs/
```

可以从仓库中的示例配置开始：

```bash
cp workspace/config.example.json workspace/config.json
```

至少需要配置 OpenAI 兼容接口，以及 Planner、Evaluator 和 Responder 使用的模型。若要为桌面前端开放 API，需要启用本地 HTTP 监听：

```json
{
  "http": {
    "listen": "127.0.0.1:8090"
  }
}
```

不要提交真实 API Key、`workspace/config.json`、SQLite 数据文件或日志。

## 启动服务

```bash
go build -o octosucker ./cmd/octosucker
./octosucker -workspace ./workspace
```

OctoSucker 是一个服务进程，不再提供终端聊天模式。使用示例 HTTP 配置时，API 地址为 `http://127.0.0.1:8090`，日志写入 `workspace/logs/agent.log`。

## Task API

提交助理输入以创建 Task：

```http
POST /api/assistant/input
Content-Type: application/json

{
  "content": "整理这批会议记录并生成摘要"
}
```

服务返回 `202 Accepted` 和初始 Task 快照：

```json
{
  "action": "task_created",
  "task": {
    "id": "...",
    "status": "running",
    "version": 1,
    "messages": [
      {
        "id": "...",
        "role": "user",
        "content": "整理这批会议记录并生成摘要",
        "created_at": "..."
      }
    ],
    "steps": []
  }
}
```

通过低频轮询读取最新快照：

```http
GET /api/tasks/{taskID}
```

Task 状态包括 `running`、`waiting_input`、`waiting_approval`、`completed`、`failed` 和 `cancelled`。

当快照中包含 `pending_interaction` 时，将表单值提交到同一个 Task：

```http
POST /api/tasks/{taskID}/interactions/{interactionID}
Content-Type: application/json

{
  "values": {
    "audience": "全体员工"
  }
}
```

处于 `waiting_input` 状态的 Task 也可以通过自由文本继续执行：

```json
{
  "active_task_id": "...",
  "content": "通知对象是全体员工"
}
```

如果 `active_task_id` 指向已经结束的 Task，本次输入会创建一个新的子 Task。当原 Task 仍在运行或等待审批时，新的自由文本输入会被拒绝。

待审批的高风险动作必须被明确处理：

```http
POST /api/tasks/{taskID}/approvals/{approvalID}
Content-Type: application/json

{
  "decision": "approved"
}
```

使用 `rejected` 拒绝操作。无论批准还是拒绝，运行时都会恢复之前暂停的执行流程，并根据审批结果继续处理。

`POST /api/chat` 仍保留给需要同步回复的集成，例如 Telegram 适配器。桌面前端应使用 Task API，因为只有 Task API 能暴露执行进度、结构化交互和风险审批状态。

## 扩展模型

新增能力时应选择以下边界之一：

1. 使用 MCP 接入结构化外部服务。
2. 使用类型化 Provider 将可执行程序适配成经过 Schema 校验的工具。
3. 使用目录式 Agent Skill 提供过程知识和工作流规则。

Skill 不是可执行协议。稳定集成不应要求模型根据文档重新拼接二进制名称、参数或 `argv`。`run_command` 只用于真正临时、无法预先建模的命令。

每个 Skill 是一个目录，其中必须包含 `SKILL.md`，也可以附带其他资源：

```text
skills/opencli/
|-- SKILL.md
`-- references/
    `-- authentication.md
```

默认工具目录只包含 Skill 的名称和描述。完整说明在 Skill 激活后才会注入上下文，其他资源按需读取。

## 验证

```bash
go test ./...
go vet ./...
```
