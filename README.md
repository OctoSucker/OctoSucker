# OctoSucker

An AI agent execution platform with a **Tool Provider** system, built for fast iteration and local-first workflows.

- 单 Agent、任务队列驱动的 **ReAct 循环执行引擎**
- 通过 `octosucker-tools` 加载的可插拔 **Tool Provider 体系**
- 通过本地 `workspace/*.md` 文件配置的 **系统提示词与启动任务**

---

## 核心架构

### AgentRuntime 与架构

顶层运行时为 **AgentRuntime**，其下为 **ToolRegistry**（本地工具）、**MCPRegistry**（MCP 工具）与 **Capability Registry**（能力图/技能编排）。详见 [docs/architecture.md](../docs/architecture.md)。

### AgentRuntime 核心

- **ReAct 循环**：Reasoning → Acting → Observing，直至任务完成或达到迭代/超时上限。
- **能力自动装配**：启动时通过 `LoadAllToolProviders` 加载各 Tool Provider，将注册的 Tool 写入 **ToolRegistry**，再经 `RegisterToolCapabilities` 映射为能力图节点，并刷新入口 capability 集合。
- **任务队列**：`Task` 通过 `SubmitTask` 进入队列，由 `runTaskQueueProcessor` 串行执行。
- **向量记忆**：
  - 使用 `agent/memory` 本地向量存储（JSONL + embedding）。
  - 在任务推理前检索相关记忆注入系统消息。
  - 工具调用结果和最终回答写回记忆。
- **LLM 集成**：
  - 通过 `agent/llm` 封装 OpenAI 兼容接口。
  - 支持 Function Calling，调用 ToolRegistry / MCPRegistry 中的工具。

### Tool Provider 系统（`github.com/OctoSucker/octosucker-tools`）

- **两套注册表**：
  - **Provider Registry**（`tool_provider_registry.go`）：维护「provider 名 → ToolProviderInfo」的全局表，表示有哪些工具**包**（每个包可提供多个 tool）。
  - **Tool Registry**（`tool_registry.go`）：内部以 `providerName/toolName` 为 key 存 Tool；对外（能力图、LLM）暴露**公开名**：无重名时用短名（如 `read_file`），重名时用全名，执行时 GetTool 支持短名解析。

- **注册与加载流程**：
  - 各工具包（如 `tools-fs`、`tools-web`、`tools-telegram`）在 `init()` 中调用：
    - `tools.RegisterToolProvider(&tools.ToolProviderInfo{ Name, Description, Provider })`
  - 主程序启动时调用：
    - `tools.LoadAllToolProviders(toolRegistry, agent, cfg.ToolProviders, rt.SubmitTask)`
  - 加载时对每个已注册的 Provider：先 `Provider.Init(config, submitTask)`，再 `Provider.Register(toolRegistry, agent, providerName)`，由各包把名下 tool 注册进 ToolRegistry；同时填充 `ToolProviderInfo.Loaded`、`InitError`。

- **ToolProvider 接口**（每个工具包实现）：
  - `Init(config, submitTask) error`：用配置与提交任务的回调做初始化。
  - `Cleanup() error`：卸载/释放资源。
  - `Register(registry, agent, providerName) error`：向 `registry` 注册该包提供的多个 Tool（`registry.RegisterTool(providerName, &Tool{...})`）。

- **工具暴露形式**：
  - 每个 Tool 包含：`Name`（短名，如 `read_file`）、`Description`、`Parameters`（JSON Schema）、`Handler(ctx, params)`。
  - 能力图与 LLM 所见工具名：无重名时用短名，多 provider 提供同名 tool 时用全名；执行时按短名或全名均可解析。

- **内建与重载**：
  - 内建能力（如 `log_message`、`list_tool_providers`、`reload_tool_provider`、`read_config_file`）由 `octosucker-tools/builtin_tool_provider` 提供。
  - 运行时可通过 `reload_tool_provider` 工具重新加载指定 Provider（先 Cleanup 再 Init + Register）。

### MCP 工具（`tools-mcp`）

- 使用 Model Context Protocol（MCP）连接外部服务。
- 启动时根据配置连接 MCP server、拉取工具列表，将每个 MCP 工具编译为普通 Tool 形态注册。
- 对 Agent 而言，MCP 能力与本地 Tool 一致，由 LLM 通过 Function Calling 调用；执行时由 MCPRegistry 处理。

---

## 配置与运行

### 配置文件（`workspace/config.json`）

- **llm**：LLM 配置
  - `baseURL`：OpenAI 兼容接口地址
  - `apiKey`：API 密钥
  - `model`：聊天模型名称
- **react**：ReAct 配置
  - `max_react_iterations`：单任务最大推理迭代次数
  - `task_timeout_sec`：单任务超时
  - `max_tool_retries`：单次工具执行失败重试次数
  - `tool_timeout_ms`：单次工具执行超时（毫秒）
  - `memory_path`：向量记忆路径（如 `workspace/memory`）
  - `embedding_model`：embedding 模型
- **tool_providers**：按 provider 名分组的配置，在 `LoadAllToolProviders` 时作为 `config` 传入各 Provider 的 `Init`。
  - 例如：`github.com/OctoSucker/tools-fs` 的 `workspace_dirs`、`github.com/OctoSucker/tools-web` 的 `search_api_key`、`github.com/OctoSucker/tools-exec` 的 `workspace_dirs`/沙箱设置、`github.com/OctoSucker/octosucker-tools/builtin` 的 `config_path` 等。

### 系统提示和启动任务（`workspace/*.md`）

- **系统提示词**：`workspace/system_prompt.md`，由 `config.LoadSystemPrompt()` 读入并传给 `NewAgentRuntime`。
- **启动任务**：`workspace/startup_tasks.md`，按 `\n---\n` 分段，每段为一条启动任务描述，在 `main.go` 中作为 `StartupTasks` 传给 `AgentRuntime.Start` 排队执行。

---

## 项目结构

```
OctoSucker/
├── README.md
├── go.mod
├── go.sum
├── main.go                    # 入口：加载配置、创建 AgentRuntime、启动任务
├── config/
│   ├── config.go              # 配置加载（含 config.json）
│   └── prompt.go              # 从 workspace/*.md 读取系统提示与启动任务
├── agent/
│   ├── runtime/
│   │   ├── agent.go           # AgentRuntime（任务队列、ReAct、Tool/MCP 调用）
│   │   ├── react.go           # ReAct 循环与工具执行
│   │   ├── task.go            # Task 与 SubmitTask
│   │   └── task_memory.go     # 任务与记忆衔接
│   ├── planner/               # 规划与能力选择
│   ├── llm/                   # LLM 客户端与 Function Calling
│   └── memory/                # 向量记忆存储
├── capability/
│   ├── registry/              # 能力图注册表（节点 = providerName/toolName 等）
│   └── workflow/              # 技能工作流与模板解析
└── workspace/
    ├── config.json            # LLM / ReAct / tool_providers 配置
    ├── system_prompt.md
    ├── startup_tasks.md
    └── memory/                # 向量记忆（自动生成）
```

外部依赖（单独仓库）：

- `github.com/OctoSucker/octosucker-tools`：Tool Provider 注册与内建工具（provider_registry、tool_registry、builtin_tool_provider）
- `github.com/OctoSucker/tools-fs` / `tools-web` / `tools-exec` / `tools-remember` / `tools-cron` / `tools-telegram` / `tools-mcp`：具体工具能力包

---

## 运行方式

```bash
cd OctoSucker/OctoSucker

go mod tidy
go run ./...
```

启动后：

- 按 `workspace/startup_tasks.md` 执行初始化任务（如检查 Tool Provider 状态、启动 Telegram 监听等）。
- 通过配置的通信渠道（如 Telegram）与 Agent 交互，由 ReAct 循环选择能力并调用 ToolRegistry / MCPRegistry 中的工具完成任务。
