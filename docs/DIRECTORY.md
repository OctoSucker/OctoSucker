# Runtime Directory And Boundaries


本文档描述当前 `agent/internal/runtime` 的最新目录架构与职责边界（与当前实现保持同步）。

## 目录结构

```text
agent/internal/runtime/
  app/
    app.go

  engine/
    dispatcher.go
    contracts.go

  cognition/
    decision/
      policy.go
    planning/
      planner.go
    evaluation/
      step_critic.go
      trajectory_critic.go
    learning/
      learner.go
    memory/
      recall_archiver.go

  execution/
    plan_executor.go
    tool_executor.go
    interfaces.go

  store/
    session.go
    skill_registry.go
    routing_graph.go
    recall.go
    capability_registry.go
```

## 分层职责边界

- `app/`
  - 运行装配入口。
  - 负责初始化依赖、组装 runtime 组件并启动。

- `engine/`（Runtime + Orchestration）
  - `dispatcher.go`: 事件循环和 `event -> handler` 分发。
  - `contracts.go`: 跨层依赖接口（Session/RouteGraph/Skill/ToolInvoker 等）。
  - 不承载 cognition / execution 的业务实现（已移出 engine）。

- `cognition/`（Brain）
  - `decision/policy.go`: Router + Policy（Skill/Graph/Heuristic/Planner）。
  - `planning/planner.go`: 用户输入到计划生成/技能计划选择。
  - `evaluation/step_critic.go`: step 级观测评估、重试与切换判定。
  - `evaluation/trajectory_critic.go`: 轨迹评估、评分与 replan 触发。
  - `learning/learner.go`: skill/graph 学习更新。
  - `memory/recall_archiver.go`: turn 结果写入 recall 记忆。

- `execution/`（Executor）
  - `plan_executor.go`: 计划推进、并行 step wave、能力切换。
  - `tool_executor.go`: 工具调用并发出 observation 事件。
  - `interfaces.go`: execution 层内部契约（session/route graph/capability）。

- `store/`（Memory）
  - 仅负责存储/检索与匹配逻辑，不负责调度编排。
  - 包含 session、skill registry、routing graph、recall、capability registry。

## 当前设计原则（已落地）

- Runtime 只调度，业务逻辑不进入事件循环。
- Brain 负责决策，Executor 负责执行，Store 负责记忆。
- Router 使用 `policies []Policy` 并按候选决策择优。
- 通过 `engine/contracts.go` 实现接口化解耦，避免具体实现耦合。
