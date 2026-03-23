# `agent/internal` 分层

`internal/` 下按 **配置 → 运行时引擎（含领域子包与流水线）→ 组装 → 测试** 组织。

与 `internal/` 并列的 **`agent/pkg/`** 为 Agent 共享库（`ports`、`mcpclient`、`llmclient`、`telegram`），**本仓库内仅 Agent 侧引用**；`mcpsvc` 不依赖该树。

## 目录与职责

| 层 | 路径 | 职责 |
|----|------|------|
| 共享库 | `agent/pkg/*` | `ports`、MCP 客户端、LLM、Telegram、追踪（相对模块根，见 `docs/DIRECTORY.md`） |
| 基础 | `internal/config` | Workspace / Capabilities JSON、路径解析 |
| 引擎 — 轨迹 | `runtime/engine`（`package engine`；`Trajectory` + **`Chat`**） | 轨迹 LLM |
| 引擎 — 流水线 | `runtime/engine`（**`Dispatcher`**、**`dispatchEvent` / `Run`**、阶段与事件；**`ports.Plan`** 状态方法在 **`pkg/ports`**）；**`runtime/store`**（**`SessionStore`**、**`RoutingGraph`**、**`RecallCorpus`**、**`CapabilityRegistry`**、**`SkillRegistry`**） | **`dispatchUserInput` / `dispatchPlanProgress` / `dispatchToolCall` / `dispatchObservation` / `dispatchTrajectoryCheck` / `recordSkillLearning` / `archiveRecall`**；技能表与 embedding 匹配；能力 ID→tools；可召回语料 |
| 组装 — 入口编排 | `runtime/app`（**`App.Dispatcher`** **`*engine.Dispatcher`**；**`RunInput` / `RunEvents`**、**`RerunSessionPlan`**、**`ErrRerunNoPlan`**、HTTP） | 与 **`Dispatcher`**、**`store.SessionStore`** 衔接 |
| 测试 | `tests/*`（含 `testdata/`） | 集中测试与夹具 |

## 依赖直觉

- `internal/config` 不依赖本仓库其它 `internal` 子包（可依赖 **`agent/pkg`**）。
- `runtime/engine` 依赖 **`runtime/store`**、**`pkg/*`** 等；**不**依赖 `runtime/app`。**`runtime/store`** **不**依赖 `engine`。
- `runtime/app` 站在最外圈，串联 **`runtime/engine`** 与 **`internal/config`**；工具执行经注入的 **`mcpclient.MCPRouter`**（通常 **MCP Client**）。Telegram 入站用 **`agent/pkg/telegram`**（`App.Telegram` 字段）。

具体 import 以代码为准；本表用于导航与 Code Review 时对齐心智模型。
