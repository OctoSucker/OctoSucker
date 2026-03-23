# OctoSuckerPlus Agent 使用说明

运行与 HTTP/代码调用速查。架构与路线图见 **`ARCHITECTURE_AND_PROGRESS.md`**，目录边界见 **`DIRECTORY.md`**、**`AGENT_INTERNAL_LAYERS.md`**。

## 目录与包（速查）

| 路径 | 作用 |
|------|------|
| `agent/cmd/octoplushttp` | 官方入口：HTTP + Telegram `RunPoll` |
| `agent/internal/runtime/app` | `App`、`NewFromWorkspace`、`HTTPHandler`；**`RunInput` / `RunEvents` / `RerunSessionPlan`**、**`ErrRerunNoPlan`** |
| `agent/internal/runtime/engine` | `package engine`：**`Dispatcher`**（上述阶段均为其方法，无独立 Planner/Executor struct） |
| `agent/internal/runtime/store` | `package store`：**`SessionStore`**、能力路由图 **`RoutingGraph`**、**`RecallCorpus`**、**`CapabilityRegistry`**、**`SkillRegistry`** / **`SkillEntry`**（`App` 经 **`Dispatcher.Sessions`** 使用会话表） |
| `agent/internal/config` | **`Workspace`**、`LoadWorkspace`、`LoadCapabilities` |
| `agent/pkg/ports` | `Session`、`Event`、`Plan` 等 DTO |
| `agent/pkg/mcpclient`（`package mcpclient`） | **`MCPRouter`**、生产 **`ConnectForApp`** |
| `workspace/`（模块根下） | **`config.example.json`** 等示例 |

在仓库模块根执行：**`cd OctoSuckerPlus`**。Go 版本见 **`go.mod`**（当前 **1.25+**）。

---

## Workspace

`-workspace` 指向的目录需含 **`config.json`**：

| 项 | 说明 |
|----|------|
| `openai` | 规划与 Embed（多实例，同一配置） |
| `mcp.endpoint` | 可多 URL，逗号分隔 |
| `http.listen` | HTTP，默认 **`:8080`** |
| `telegram.bot_token` / `telegram.default_chat_id` | 入站 **`RunPoll` 始终启用**；可选 **`allowed_chat_ids`** |

**`default_agent_capabilities.json`**：仅 **`config.LoadCapabilities`** 等静态加载用；**`NewFromWorkspace`** 走 MCP **`list_tools`** 推断能力，不写该文件。

示例：`cp workspace/config.example.json /path/to/ws/config.json` 后编辑。

```bash
 go run ./ -workspace ./workspace
```

**`POST /run`**（端口以 `http.listen` 为准）：

```bash
curl -s -X POST http://127.0.0.1:8080/run -H 'Content-Type: application/json' \
  -d '{"session_id":"job1","text":"…"}'
```

**健康检查**：`GET /health` → `ok`。

**Telegram 入站**：`agent/pkg/telegram`，会话 id **为 `chat_id` 的十进制字符串**（与 HTTP 的 `http-…` 前缀规则区分）；与 MCP **`mcp-telegram`**（发消息工具）是两回事。

**多 MCP**：`mcp.endpoint` 写多个 URL。示例：Telegram MCP `go run ./mcpsvc/telegram/cmd/mcp-telegram -listen :8765`；Exec `EXEC_WORKSPACE_DIRS=… go run ./mcpsvc/exec/cmd/mcp-exec -listen :8766`。细节见 **`mcpsvc/README.md`**、**`.env.example`**。

---

## HTTP API（JSON）

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/health` | 存活 |
| POST | `/run` | 一轮用户输入 |
| GET | `/session/{id}` | 会话详情 |
| POST | `/session/{id}/rerun` | 用已有 Plan 重跑；无 plan 时 **`ErrRerunNoPlan`**（404） |

**`POST /run`** 体：`{"session_id":"…","text":"…"}`。若 `session_id` **不以** **`http-`** 开头，服务端会规范为 **`http-` + 原值**（响应里的 **`session_id`** 为规范后的值；后续 **`GET /session/…`**、**`rerun`** 须用同一字符串）。响应含 **`reply`**、**`trajectory_score`**、**`trajectory_summary`** 等。

**会话**：HTTP 侧为 **`http-…`**；Telegram 侧为 **`chat_id` 字符串**，二者命名空间不重叠。**首次** **`POST /run`**（或 Telegram 首条消息）时 **`dispatchUserInput`** 若查无该 id 会 **新建** **`ports.Session`** 再规划；存储为内存 **`SessionStore`**（经 **`Dispatcher.Sessions`**），重启丢失。

---

## 默认行为（摘要）

- **规划**：须 **`Dispatcher.PlannerLLM`**（**`llmclient.OpenAI`**）；Skill 优先（**`PlanFor` / 向量阈值**），否则 LLM 必须返回可解析 plan JSON；Embed 失败报错。
- **Skill 表**：**`store.NewSkillRegistry()`** 默认为空；由 **`recordSkillLearning`**（**`MergeOrAdd`**）或 **`Register`** 写入 **`SkillEntry`**。
- **MCP 工具参数**：由规划阶段在计划 JSON 里为每步写可选 **`arguments`**（与 **`mcpclient.MCPRouter`** 启动时 **`list_tools`** 得到的 **Input Schema** 一致，经 **`PlannerToolAppendix`** 进系统提示）。执行器 **不再**按工具名硬编码参数。会话 id **为纯十进制整数串**（Telegram 的 `chat_id`）且 **非** **`http-` 前缀** 时，用户提示里会附带当前 **`chat_id`**，便于模型填 **`get_telegram_chat`** 等所需字段。
- **执行**：**`mcpclient.MCPRouter`** 解析 capability → tool 并 **`Invoke`**。

**`llmclient.OpenAI`**：基于 **`github.com/openai/openai-go`**；`model` / `embeddingModel` 非空，空响应报错。

---

## 代码中嵌入 `App`

模块：`github.com/OctoSucker/agent/internal/runtime/app`。

```go
import (
    "context"

    "github.com/OctoSucker/agent/internal/config"
    "github.com/OctoSucker/agent/internal/runtime/app"
)

cfg, err := config.LoadWorkspace("/path/to/ws")
if err != nil { panic(err) }
ctx := context.Background()
a, err := app.NewFromWorkspace(ctx, cfg)
if err != nil { panic(err) }
defer a.Close()

reply, err := a.RunInput(ctx, "http-sid-1", "hello")

// 多事件入队：
// import "github.com/OctoSucker/agent/pkg/ports"
// replies, err := a.RunEvents(ctx, []ports.Event{ ... })
```

与 **`octoplushttp`** 一致时，另起 **`a.HTTPServer.ListenAndServe`**、**`a.Telegram.RunPoll`**（见 **`agent/cmd/octoplushttp/main.go`**）。

---

## 测试

```bash
cd OctoSuckerPlus && go test ./...
```

---

## 相关文档

| 文档 | 内容 |
|------|------|
| `ARCHITECTURE_AND_PROGRESS.md` | 架构与完成度 |
| `DIRECTORY.md` | 目录与 import 速记 |
| `AGENT_INTERNAL_LAYERS.md` | `internal` 分层 |
