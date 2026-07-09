# OctoSucker

一个用 Go 实现的 AI agent runtime。

当前版本的核心重点不是“知识图直接驱动 agent”，而是先把 agent 主链本身做稳定：

- 明确的 runtime 状态机
- 可学习的 tool routing graph
- OpenAI 兼容模型做 step planning 和 trajectory evaluation
- 内置工具 + MCP 工具统一注册到同一层工具目录
- workspace 作用域的 SQLite 持久化

知识图组件已经迁移为独立外部工具，但不作为 agent 主决策闭环的一部分。

## 当前架构

主链路是一个单回合、单任务的事件循环：

```text
TurnRequested
  -> Planner
  -> StepScheduled
  -> PlanExecutor
  -> StepObserved
  -> StepEvaluator
  -> TrajectoryEvaluationRequested
  -> TrajectoryEvaluator
  -> nil | TurnRequested
```

更完整的状态机说明见 [internal/runtime/DESIGN.md](/Users/zecrey/Desktop/OctoSucker/OctoSucker/internal/runtime/DESIGN.md)。

## 目录

| Path | Responsibility |
| --- | --- |
| `cmd/octosucker` | 进程入口，只负责 flag、config、runtime、gateway 装配 |
| `config` | workspace config 加载和路径解析 |
| `internal/runtime` | agent runtime 状态机 |
| `internal/runtime/toolrouting` | agent 使用的 learned tool routing graph |
| `internal/runtime/planning` | route decision、step selection、argument generation |
| `internal/runtime/execution` | step execution |
| `internal/runtime/judge` | step / trajectory evaluation |
| `internal/runtime/model` | task、plan、event、payload、tool result |
| `internal/runtime/taskstore` | 进程内 task state |
| `internal/tools` | builtin tools、MCP sessions、registry |
| `internal/storage` | SQLite schema 和 persistence helpers |
| `internal/gateway` | ingress 装配 |
| `internal/ingress` | stdin、Telegram、admin HTTP |
| `pkg/llmclient` | OpenAI 兼容客户端封装 |
| `pkg/workspacelog` | workspace 日志文件工具 |
| `workspace` | 本地示例 workspace 资源：`config.example.json`、sandbox profile、skills |

## Workspace 约定

`-workspace` 指向一个已存在的 agent home。

这个目录通常包含：

- `config.json`
- `skills/`
- `knowledge/`
- `data/`
- `logs/`

运行时会把它当成 agent 自己的工作区根目录。  
`exec.workspace_dirs` 只表示命令执行允许访问的目录，不再承担 agent root 语义。

## 快速开始

1. 准备一个 workspace 目录。
2. 复制示例配置：

```bash
cp workspace/config.example.json /your/workspace/config.json
```

3. 修改 `config.json`，至少填这些字段：

- `openai.api_key`
- `openai.base_url`
- `openai.model`
- `openai.embedding_model`

4. 构建并运行：

```bash
go build -o octosucker ./cmd/octosucker
./octosucker -workspace /your/workspace
```

也可以直接：

```bash
go run ./cmd/octosucker -workspace /your/workspace
```

## 当前设计取向

当前代码刻意偏向：

- 单回合编排容易读
- `Task` 聚合根集中持有回合状态
- `Plan` 集中持有步骤序列状态
- 用渐进收口替代一次性大拆分

当前代码不追求：

- 高度泛化的 workflow engine
- 多任务并发图执行
- 过度抽象的 actor / bus 框架

这是有意选择，不是未完成状态。

## 清理约定

这些文件不应进入仓库：

- 本地二进制
- `.gocache/`
- `workspace/data/`
- `workspace/logs/`
- 带真实密钥的 `workspace/config.json`

仓库里只保留 `workspace/config.example.json` 作为示例。

## Knowledge Graph Tool

`knowledge_graph` 已经从 agent 逻辑里迁移成外部独立仓库工具。

它提供两种接入方式：

- CLI: 外部 `kggraph` 二进制
- MCP stdio server: 外部 `kggraph serve-mcp`

当前仓库不再为知识图插件保留专用 builtin wrapper。推荐做法是安装外部 `kggraph`，再通过 skill 把它作为 CLI 插件接入；另一种做法是直接把 `kggraph serve-mcp` 作为 MCP endpoint 接入。

`workspace/skills/` 不再默认包含 KGgraph CLI plugin。需要 KGgraph 时，在 workspace 自己的 `skills/` 中添加 CLI plugin 配置，或把 `kggraph serve-mcp` 暴露为 MCP endpoint。
