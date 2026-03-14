# OctoSucker

An AI agent execution platform with a **Tool Provider** system and **Skills registry**, built for fast iteration and local-first workflows.

- 单 Agent、任务队列驱动的 **ReAct 循环执行引擎**
- 通过 `octosucker-tools` 加载的可插拔 **Tool Provider 体系**
- 通过 `octosucker-skills` 提供的 **Skill Registry（SKILL.md 解析器）**
- 通过本地 `workspace/*.md` 文件配置的 **系统提示词与启动任务**

---

## 核心架构

### Agent 核心

- **ReAct 循环**：Reasoning → Acting → Observing，直至任务完成或达到迭代/超时上限。
- **任务队列**：`agent.Task` 通过 `SubmitTask` 进入队列，由 `runTaskQueueProcessor` 并发执行。
- **向量记忆**：
  - 使用 `agent/memory` 本地向量存储（JSONL + embedding）。
  - 在任务第一轮推理前检索相关记忆注入系统消息。
  - 工具调用结果和最终回答写回记忆。
- **LLM 集成**：
  - 通过 `agent/llm.LLMClient` 封装 OpenAI 兼容接口。
  - 支持 Function Calling，用于调用 Tool Provider 暴露的工具。

### Tool Provider 系统（`github.com/OctoSucker/octosucker-tools`）

- **统一注册机制**：
  - 每个工具包（如 `tools-fs`、`tools-web`、`tools-exec`、`tools-mcp` 等）在 `init()` 中调用：
    - `tools.RegisterToolProviderWithMetadata(...)`
  - `OctoSucker` 主程序通过：
    - `tools.LoadAllToolProviders(toolRegistry, agent, cfg.ToolProviders)`
    加载并初始化所有 Tool Provider。
- **生命周期接口**：
  - 可选实现 `ToolProviderLifecycle`：
    - `Init(config map[string]interface{}) error`
    - `Cleanup() error`
  - 支持运行时通过工具触发 `reload_tool_provider` 等能力（见 `octosucker-tools/builtin.go`）。
- **工具暴露形式**：
  - 每个 Tool 为：
    - `Name`：工具名（如 `read_file`、`web_fetch`、`mcp_exa_search`）
    - `Description`：LLM 可读说明
    - `Parameters`：JSON Schema 风格的参数定义
    - `Handler(ctx, params)`：Go 实现

### Skills Registry（`github.com/OctoSucker/octosucker-skills`）

- **目标**：兼容 Claude Code / Cursor 的 `SKILL.md` 标准，支持通过 Go 模块统一解析和管理技能定义。
- **能力**：
  - 扫描指定目录（默认 `workspace/skills/`）下的 `SKILL.md`。
  - 解析 YAML frontmatter 与正文，生成：
    - `Skill{Name, Description, Body, Path, Metadata}`
  - 通过：
    - `skills.NewRegistry()`
    - `(*Registry).LoadFromDirs(dirs []string)`
    - `(*Registry).List()` / `Get(name)`
    向 Agent 暴露统一接口。
- **与 Agent 的关系**：
  - `Agent` 持有 `skillRegistry *skills.Registry`。
  - 后续可通过 Tool Provider 暴露工具如 `skill.list`、`skill.invoke`，让 LLM 主动选择并应用技能。

### MCP 工具（`tools-mcp`）

- 使用 Model Context Protocol（MCP）连接外部服务（如 Exa、Context7 等）。
- 启动时根据配置的 servers：
  - 连接 MCP server，调用 `ListTools`。
  - 将每个 MCP 工具编译成普通 Tool（如 `mcp_exa_search`）。
  - 额外提供元信息工具 `mcp_list_servers`、`mcp_*_meta`。
- 对 Agent 来说，MCP 能力完全以“普通工具”形式出现，由 LLM 通过 Function Calling 调用。

---

## 配置与运行

### 配置文件（`config/agent_config.json`）

- **llm**：LLM 相关配置
  - `baseURL`：OpenAI 兼容接口地址（如 DashScope）
  - `apiKey`：API 密钥
  - `model`：聊天模型名称
- **react**：ReAct 相关配置
  - `max_react_iterations`：单任务最大推理迭代次数
  - `task_timeout_sec`：单任务超时时间
  - `memory_path`：向量记忆存储路径（如 `workspace/memory`）
  - `embedding_model`：embedding 模型名称
- **tool_providers**：每个 Tool Provider 的配置（按 provider 名字分组）
  - 例如：
    - `github.com/OctoSucker/tools-fs`：`workspace_dirs` 白名单
    - `github.com/OctoSucker/tools-web`：`browser_proxy`、`browser_headless` 等
    - `github.com/OctoSucker/tools-exec`：命令白名单/黑名单、沙箱设置
    - `github.com/OctoSucker/tools-mcp`：MCP servers 列表

### 系统提示和启动任务（`workspace/*.md`）

- **系统提示词**：`workspace/system_prompt.md`
  - 内容为 Agent 的系统级指导（如何区分任务/查询/闲聊、如何使用工具、remember_* 原则、定时任务前缀、MCP 工具说明等）。
  - 在启动时通过 `config.LoadSystemPrompt()` 读入并传给 `agent.NewAgent`。
- **启动任务**：`workspace/startup_tasks.md`
  - 按 `\n---\n` 分隔的多段 Markdown，每一段是一个启动时要执行的初始化任务描述。
  - 在 `main.go` 中作为 `StartupTasks` 传给 `agent.Start`，由 Agent 以普通任务形式排队执行。

---

## 项目结构（当前）

```
OctoSucker/
├── README.md
├── LICENSE
├── go.mod
├── go.sum
├── agent/
│   ├── agent.go        # Agent 核心（任务队列、ReAct 循环）
│   ├── react.go        # ReAct 逻辑与工具调用
│   ├── task.go         # Task 定义与提交
│   ├── llm/            # LLM 客户端封装
│   └── memory/         # 向量记忆
├── config/
│   ├── agent_config.json  # LLM / ReAct / Tool Provider 配置
│   ├── config.go          # 配置加载
│   └── prompt.go          # 从 workspace/*.md 读取系统提示与启动任务
├── workspace/
│   ├── system_prompt.md   # 系统提示词
│   ├── startup_tasks.md   # 启动任务
│   └── memory/            # 向量记忆存储（自动生成）
└── main.go             # 程序入口（加载配置、创建 Agent、启动任务）
```

外部依赖仓库（单独 Git 仓库）：

- `github.com/OctoSucker/octosucker-tools`：Tool Provider 核心与内建工具
- `github.com/OctoSucker/octosucker-skills`：Skill Registry（SKILL.md 解析）
- `github.com/OctoSucker/tools-fs` / `tools-web` / `tools-exec` / `tools-remember` / `tools-cron` / `tools-mcp` / `tools-telegram`：具体工具能力

---

## 运行方式

```bash
cd OctoSucker/OctoSucker

# 确保 go.mod 可拉到所有依赖
go mod tidy

# 运行 Agent（使用默认配置）
go run ./...
```

启动后：

- Agent 会按 `workspace/startup_tasks.md` 描述执行一系列初始化任务（检查 Tool Provider 状态、启动 Telegram 监听、汇报 MCP 能力等）。
- 后续可以通过配置的通信渠道（如 Telegram）与 Agent 交互，由 Agent 使用 ReAct 循环自动选择工具完成任务。

---
