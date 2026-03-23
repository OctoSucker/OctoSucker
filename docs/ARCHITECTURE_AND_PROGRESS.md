# OctoSucker Agent：架构与开发进展

本文档以 **流水账式** 盘点 Agent 整体方案，逐条标注完成进度。对照实现为 **OctoSuckerPlus**（`OctoSuckerPlus/`）。

**代码目录**：**`agent/`** 与 **`mcpsvc/`** 并列：Agent 侧 **`agent/cmd`**、**`agent/pkg`**（`ports`、`mcpclient`、`llmclient`、`telegram` 等共享库）、仓库根 **`workspace/`**（示例配置）、**`agent/internal`**（仅 `agent/` 子树可 import；子目录按 **`config` / `runtime` / `tests`** 分层：**`runtime/store`**（内存：**`SessionStore`**、能力路由图 **`RoutingGraph`**、**`RecallCorpus`**、**`CapabilityRegistry`**、**`SkillRegistry`**）、**`runtime/engine`**（`package engine`：**`Dispatcher`** 等），见 **`docs/AGENT_INTERNAL_LAYERS.md`**）。**`mcpsvc/`** 为独立 MCP 服务（**`registry/`**、各 `cmd/mcp-*` 等）。

---

## 任务队列（Ingress Task Queue）— 评估与结论

多入口（HTTP、Telegram 等）若各自直调 `RunInput`，高并发时易把 LLM/MCP 打满；**Ingress 队列**（在入口与 `RunInput` 之间）可做缓冲、背压、顺序与监控。

**与 `engine.Dispatcher` / `Run` 的关系**：**正交**。**`Dispatcher.Run`** 是 **单轮 turn 内** 的事件链；Ingress 是 **跨请求** 的「用户回合」层。两者不互相替代。

**当前代码**：**未实现** Ingress 队列；`POST /run`、Telegram `RunPoll` 均 **直接** `RunInput`。`POST /session/{id}/rerun` 等路径亦直接调 app。实现位置与形态后续再定。

---

## 当前阶段结论

- **Agent OS**：事件驱动、Plan DAG、Capability、ToolGraph、双层 Critic、Memory、Skill、Learning Loop、Persist、HTTP 等 **骨架已闭环**；定位是 **Agent 操作系统 + Learning Runtime**，不是「仅 LLM + 平铺 Tool」的 Agent SDK。
- **Learning Control**：流水账 36–39（Skill 治理、轨迹信用分配、Policy Learner 数据通路、探索衰减）**已落地**；40–41 仍为可选/远期。
- **下一阶段重心（已拍板范式）**：**与 Agent 解耦的 MCP-only 工具层**——**全部业务工具**按 **MCP 规范**实现，由 **MCP 服务**注册与管理；**Agent 进程不实现具体工具**，仅在执行链上作为 **MCP Client** 调用 `list_tools` / `call_tool`。**工具热插拔**：增删改工具只需 **MCP 服务**侧发布/重启（若 MCP 实现支持动态注册则不必重启 Agent）；**Agent 服务不因新工具而重编译重启**。**决策链不变**：仍为 **Planner / Skill → Capability（及 ToolGraph 路由）→ 解析出 MCP 上的 tool 名 → MCP 执行**。

---

## 工具范式：Agent 与 MCP 解耦（意图说明）

| 层级 | 职责 | 是否随新工具重编 Agent |
|------|------|-------------------------|
| **Agent（OctoSuckerPlus）** | UserInput、Plan、Capability、ToolGraph、Dispatch、Policy、Critic、Memory、Skill、Learning… | **否** |
| **MCP Server（独立进程/服务）** | 实现并注册全部 Tool（MCP 标准）；工具清单与实现细节只在此侧演进 | 仅 MCP 侧部署/重启（或动态注册） |

**执行路径（目标形态）**：

**`dispatchPlanProgress`** 绑定 capability → tool **名称**（与今日一致）→ **`dispatchToolCall`** → **`MCPRouter.Invoke`**（**`mcpclient`** 经 **`NewFromWorkspace`**，或测试桩 **`apptest.NewFromLocalCapabilities`**）→ **`ports.ToolResult.Observation`** → **`EvObservationReady`** → **`dispatchObservation`**。

**与 Capability 的关系**：Capability 仍是 **Agent 内**的抽象与路由单元；其下的 `tools: ["…"]` 应对齐 **MCP Server 暴露的 tool 名称**（或由同步/缓存的 MCP `list_tools` 校验）。**选哪条 capability、走哪条 Graph 边，仍是当前模式**；变的是 **最后一步执行从本进程函数表 → 跨进程 MCP**。

**启动**：**`app.NewFromWorkspace`** 经 **`config.json`** 连 MCP 并 **`NewOpenAI`**；无 MCP 用 **`apptest.NewFromLocalCapabilities`**（**`apptest/wire_local_app.go`**）；测试 **`StubCapabilities`**。

---

## 关注点说明

- **已过去式**：曾强调「外部能力不是架构重点」——指的是 **OS 层不绑定具体浏览器/终端实现**；执行面通过 **`mcpclient.MCPRouter`**（`agent/pkg/mcpclient`）可插拔（**MCP Client** 或注入实现）。
- **当前重点**：在 **不改动** Capability / ToolGraph 核心语义的前提下，把 **工具执行** 收敛为 **`mcpclient.MCPRouter`**（生产侧首选 MCP；见流水账 42–44）。

---

## 演进路线图（能力抽象 → 决策 → 生态）

原独立稿 `AGENT_EVOLUTION_ROADMAP.md` 已并入本节；目录速查仍见 `docs/DIRECTORY.md`。

### 一、结构评价（结论）

- `agent/` 内 **config / runtime（`engine` 包）** 分层 → 方向正确。
- **MCP** 与 Agent 分离（`agent/pkg/mcpclient` + `mcpsvc/*`）→ 正确。
- **`agent/pkg/ports`** 为 **领域 DTO / 载荷**（`Session`、`Event`、`Plan` 等）；**工具执行** 由 **`mcpclient.MCPRouter`**（`ListCapabilities` + `Invoke`）承担；**内存存储** 在 **`runtime/store`**（**`SessionStore`**、能力路由图 **`RoutingGraph`**、**`RecallCorpus`**、**`CapabilityRegistry`**、**`SkillRegistry`** / **`SkillEntry`**）；**`llmclient`**、`mcpclient`、**`engine`** 流水线为可插拔实现侧。
- 主要风险在 **边界是否够硬**：capability / skill / graph 语义是否一致、**路由谁说了算** 是否可演化。

### 二、四类升级与实现状态

| # | 主题 | 状态 | 实现要点 |
|---|------|------|----------|
| 1 | **Capability Runtime** | ✅ | **`mcpclient.MCPRouter`**（`ListCapabilities` + `Invoke`）；**`Runner`** 为链内单连接实现 |
| 2 | **Skill → Graph 路径** | ✅ | **`SkillEntry.Path`**（embedding 命中时 **`MatchScore`**）；`Session.SkillPreferredPath`；Dispatch 按 **`RouteMode`** 调整 frontier 与 skill 路径的优先顺序 |
| 3 | **路由 Policy（skill / graph / planner）** | ✅ | `Session.RouteMode`、`RoutePolicy`；**`Dispatcher`**：`SkillRouteThreshold`（默认 **0.9**）、`GraphRouteThreshold`（默认 **0.7**）；HTTP `GET /session/{id}` 暴露 `route_mode` / `route_policy` |
| 4 | **MCP 插件注册（生态骨架）** | ✅ | `mcpsvc/registry`：`Registry`、`Plugin` 描述符、`LoadFile` / `ParseJSON` |

### 三、可选目录重构（接口稳定后再动）

| 建议 | 含义 |
|------|------|
| `core/` 与 `agent/` 分离 | 内核 vs 组装 |
| `provider/` 物理目录 | 与 `mcpclient` 并列表达「供给面」 |
| `routing` 下 **routing policy** | 指 skill / graph / planner 路由决策，不等于工具权限控制 |
| 按阶段拆 planner / executor / critic | handlers 膨胀后 |

### 四、执行顺序（回顾）

1. Capability 工具执行面 **`mcpclient.MCPRouter`**（已完成）  
2. Skill 路径与 Frontier 语义（已完成）  
3. 路由 Policy（已完成）  
4. MCP 注册表骨架（已完成）；动态插件加载（go plugin / wasm 等）仍为后话  

---

## 定位与原则（前置）

- **定位**：面向 Go 的生产级、自进化、多 Agent 协作系统；能力可插拔（LLM、Tool、Memory、Sandbox 等经 ports 注入）。
- **原则**：接口驱动、事件驱动、多 Agent 分工、Capability 优先、Sandbox First、可学习系统；架构完备性优先于具体接入了哪些外部能力。

---

## 与典型 Agent SDK（以 Eino 为参照）的差异与可借鉴点

**对照目的**：明确「Agent SDK」与「本仓库 Agent OS」的分工，避免用 SDK 标准误判 OS 是否完成。

| 维度 | Eino 等（ToolsNode 范式） | OctoSuckerPlus（当前） |
|------|---------------------------|-------------------------|
| Tool 形态 | `BaseTool` / `InvokableTool` + **ToolsNode** 聚合执行 | `CapabilityInvocation` + **`MCPRouter.Invoke`**（生产 **MCP**；测试可注入 **`MCPRouter` 桩**） |
| 工具来源 | 本地 + **MCP 动态拉取** | **生产**：业务工具经 **MCP**；无内置进程内假工具表 |
| 路由 | 主要依赖 LLM 选 tool | **Capability** + **ToolGraph（可学习）** + Skill / Planner |
| 学习 / 记忆策略 | 非本类框架核心 | **Graph / Skill / Trajectory** 已闭环 |
| Schema 给 LLM | **ToolInfo** 等强类型 | 以 Plan JSON + capability 枚举为主；**待加强**（见流水账 45） |

**可借鉴（不改变 OS 层级）**：

1. **执行隔离层**：本系统目标为 **MCP Client** 作为唯一业务执行出口（类 ToolsNode 的聚合点可落在 Client 内：连接池、超时、重试、多 MCP 路由）。
2. **MCP**：与拍板范式一致——**工具定义与热插拔边界在 MCP Server**，Agent 只消费协议。
3. **强类型 Tool Schema**：MCP 工具自带 `inputSchema` 等；Agent 侧应用其刷新 Planner / capabilities 配置 / 校验逻辑（流水账 45）。

**结论**：本系统在 **Capability + ToolGraph + 学习** 上区别于单层 SDK；工程上需补齐 **MCP Client 执行路径**，使 Agent 与工具实现 **进程级解耦**。

---

# Agent 整体方案流水账（含完成进度）

**图例**：✅ 已完成 | 🟡 可选/部分 | ❌ 待做

---

1. **入口层**：cmd 提供 HTTP（**`octoplushttp`**）；**`POST /run`** 与 Telegram **`RunPoll`** 均 **直接** **`RunInput`**。app 组装 **单一** **`engine.Dispatcher`**（**`NewDispatcher`**；规划 / 计划推进 / 工具 / 评判 / 学习 / 召回均为 **`Dispatcher` 方法**）。  
   **进度**：✅

2. **事件类型**：UserInput、PlanCreated、ToolsBound、ToolCall、ObservationReady、StepCompleted、StepCapabilityRetry、TrajectoryCheck、TurnFinalized。  
   **进度**：✅

3. **事件流**：`Event → Handler → []Event`；事件入队，由 **`Dispatcher.Run`** 按 **`MaxSteps`** 消费并派发到对应 Handler。  
   **进度**：✅

4. **Planner**：接收 UserInput，产出 Plan（Steps、DependsOn）；可结合 Skill 匹配结果注入已有 Plan 与 SkillPriorCaps。  
   **进度**：✅

5. **Plan 状态（`pkg/ports` 上 `*Plan` 方法）**：Plan 为 DAG；**`Runnable`**、**`MarkRunning`**、**`MarkDone`**、**`MarkPending`**、**`AllDone`**；支持按拓扑并行 wave 执行。  
   **进度**：✅

6. **Capability 注册表**：Registry 维护 Capability ID → Tools（可多 tool）；Dispatch 按 Frontier + SkillPrior 解析出当前 step 的 capability，多 tool 时按链执行。  
   **进度**：✅

7. **Contextual ToolGraph - 存储与接口**：转移模型 `(RoutingContext + lastCapability) → nextCapability`；RoutingContext 含 TaskType、IntentText、Embedding、Cost、Latency；接口 Frontier(ctx, rc, last, outcome)、RecordTransition(ctx, rc, from, to, outcome)、EntryNodes；当前实现为内存 **`store.RoutingGraph`**。  
   **进度**：✅

8. **Contextual ToolGraph - 决策**：Frontier 按边成功率 + 相似意图历史 + Cost/Latency 排序选边；近期转移 ring（recentTransitions）参与选边；相似意图用 Embedding 与历史边匹配（similarIntentScoreLocked）。  
   **进度**：✅

9. **ToolGraph 策略化（探索）**：Frontier 引入探索项（如 UCB/bandit），score = f(success, similarity, cost, latency, exploration)，避免局部最优、促进探索新路径。  
   **进度**：✅

10. **`dispatchPlanProgress`**：在 Plan 的 step 上根据 ToolGraph Frontier 与 SkillPrior 解析出 capability，绑定 Tools 后发出 **`EvToolCall`**。  
    **进度**：✅

11. **`dispatchToolCall`**：收到 **`EvToolCall`** 后通过 **`MCPRouter.Invoke`** 得到 **`ports.ToolResult`**，经 **`Observation()`** 直接发 **`EvObservationReady`**。  
    **进度**：✅

12. **`dispatchObservation`**：对单步结果做 accept / retry / switch_capability；同 tool 有限次重试；换 Capability 时发 StepCapabilityRetry，重新走 **`dispatchPlanProgress`**。  
    **进度**：✅

13. **`dispatchTrajectoryCheck`**：EvaluatePlan(plan, trace) → Score；低分或失败可触发 replan（**无**会话快照 rollback）。  
    **进度**：✅

14. **Memory**：**`store.RecallCorpus`** — 内存文本条 + 可选 embedding；`Write` / `Recall`（**`*llmclient.OpenAI`** 经 **`Embed`**）。  
    **进度**：✅

15. **Skill 注册表**：Entry 含 Name、Keywords、Capabilities、Plan、TriggerEmbedding；Match、PlanFor、MatchByEmbedding；匹配后返回 Hit(Plan, Capabilities)，供 Planner 注入 Plan 与 SkillPriorCaps。  
    **进度**：✅

16. **Skill 与 Planner 联动**：Planner 使用 **`*OpenAI`** 的 **`Embed`** 做 MatchByEmbedding，优先 embedding 匹配；命中后注入 Skill 的 Plan 与 Capabilities 优先级。  
    **进度**：✅

17. **Skill 执行主路径**：用户输入后若 Skill 高置信匹配则直接向执行引擎注入 Plan（可跳过 LLM）；并写入 **SkillPreferredPath** 影响 Dispatch 选 capability 顺序。  
   **进度**：✅（`RouteMode=skill`：embedding ≥ `SkillRouteThreshold` 或关键词 Skill；否则 graph / planner 分支见流水账 26、38）

18. **`recordSkillLearning`**：**`EvTurnFinalized`** 时 **`RecordTurn`**（关键词命中更新；否则对用户输入 **Embed** 后与 **`TriggerEmbedding`** 余弦最近邻且 ≥ **`SkillRouteThreshold`** 的一条更新 **Attempts/Successes**）、RouteGraph 轨迹、`MergeOrAdd` 等。  
    **进度**：✅

19. **`archiveRecall`**：**`EvTurnFinalized`** 时将会话 **`Reply`** 写入 **`RecallCorpus`**（供后续 Recall），非磁盘持久化。  
    **进度**：✅

20. 工具执行不再经过独立 Policy 层，直接由 MCP Provider 执行。  
    **进度**：✅

21. **`mcpclient.MCPRouter`**：`ListCapabilities` + `Invoke(inv)` 统一工具执行面。生产经 **`ConnectForApp`** → **`MCPRouter`**（**`NewFromWorkspace`**）；无 MCP 时用 **`apptest.NewFromLocalCapabilities`**（流水账 42）。  
   **进度**：✅

22. **Learning Loop - 记录**：执行后调用 RecordTransition，写入边统计、近期转移、Cost/Latency，供下次 Frontier 使用。  
    **进度**：✅

23. **Learning Loop - 选边**：Frontier 使用 RecordTransition 写入的成功率、相似意图、Cost/Latency 对候选边排序，使「执行→记录→下次选边更优」闭环。  
    **进度**：✅

24. **Learning 改写系统 - Graph**：边权/统计由学习结果更新（与 RecordTransition 协同），Frontier 选边被学习结果改写；可选整链/聚合更新。  
    **进度**：✅（按步 RecordTransition 已有；Frontier 已用 UCB 探索项）

25. **Learning 改写系统 - Skill**：从成功路径自动抽取 Skill、合并相似 Skill、按 success_rate/coverage 排序参与匹配；TurnFinalized 时写入。  
    **进度**：✅（TurnFinalized 时 success 且 score≥阈值则 BuildEntryFromSession + MergeOrAdd；MatchByEmbedding 按相似度排序）

26. **Policy Learner（路由）**：**embedding Skill ≥ SkillRouteThreshold（默认 0.9）** → `RouteMode=skill`；**否则 Graph.Confidence ≥ GraphRouteThreshold（默认 0.7）** → `RouteMode=graph`（仅 LLM 产 Plan）；**否则关键词 Skill** → skill；否则 **`RouteMode=planner`**。  
   **进度**：✅（`Session.RoutePolicy` / `RouteMode`；**`Dispatcher.SkillRouteThreshold` / `GraphRouteThreshold`** 可调）

28. **Workspace 全量快照（原 `archive/persist`）**：已移除 `PersistAll` / `RunPeriodic`（异步定时写盘易与主进程状态不一致）；会话/Graph/Skill 等落盘由后续方案接入。  
    **进度**：❌ 已删（待替代）

30. **事件 JSON 编解码**：**`ports.EventsFromJSON` / `ports.EventsToJSON`**（测试与序列化）；**无** HTTP 重放入口。  
    **进度**：✅

31. **HTTP API**：POST /run、GET /session、POST /session/{id}/rerun 等。  
    **进度**：✅

32. **包/路径与验收**：核心包 `agent/internal/runtime/app`（含 HTTP 路由）、**`agent/internal/runtime/engine`**、**`agent/internal/runtime/store`**（会话表 / 工具图 / 召回语料）、`agent/pkg/ports`（含事件 JSON）、`agent/pkg/telegram`（Bot 入站）；验收 `cd OctoSuckerPlus && go test ./...`。  
    **进度**：✅

33. **Multi-Agent 真解耦**：独立 SubAgent、按事件订阅的决策边界、多 Agent 物理隔离。  
    **进度**：🟡 可选

34. **Policy 系统化**：PolicyEngine.Evaluate(ctx, ToolCall, Context) 统一决策（风控、审计、多级审批）。  
    **进度**：🟡 可选

---

## 收敛与策略控制（流水账 36–41）

目标：让学习从「自动记录」→「受控优化」，防止 Skill 膨胀、Graph 噪声、策略震荡。

36. **Skill 质量**：用 **Successes/Attempts** 得 **成功率**（**`SkillEntry.SuccessRate()`**）；**`tooPoorForMatch`**（尝试 >5 且率 <0.3）时不出现在 Match / MatchByEmbedding / KeywordPlanEntry；**`LastUsedAt`** + **`MarkUsed`** 仍保留。  
   **进度**：✅

37. **ToolGraph 信用分（Trajectory Credit Assignment）**：按轨迹回溯更新边权（延迟奖励）；记录本轮 path (from→to)，TurnFinalized 时按 gamma 衰减对路径上每条边做加权 Success/Failure，使 Graph 学会「哪一步真正重要」。  
   **进度**：✅（Session.TransitionPath、RecordTrajectory(path, score, success)、gamma=0.9 回溯）

38. **Policy Learner（落地）**：同上第 26 条；与 **Dispatch 路由顺序**（`RouteGraph` 时 frontier 优先）及 **SkillPreferredPath** 协同。  
   **进度**：✅

39. **Exploration 衰减（Exploration Decay）**：Frontier 探索项随 totalRuns/sessionCount 衰减（如 explorationWeight = base * exp(-λ*totalRuns) 或 sessionCount>N 时降为固定小值），使系统后期收敛、策略稳定。  
   **进度**：✅（Store.totalRuns、RecordTrajectory 时自增、Frontier 中 explorationWeight *= exp(-0.001*totalRuns)，下限 0.02）

40. **Strategy 级学习（可选）**：统计各策略（skill/graph/planner）成功率，动态调整阈值或路由。  
   **进度**：🟡 可选

41. **统一价值函数（远期）**：Value(task, strategy) → expected reward，argmax_strategy 选路；RL/Bandit/Meta-controller 方向。  
    **进度**：🟡 远期

---

## 工具运行时与生态（流水账 42–47）

目标：**Agent 与工具实现解耦**；**全部业务工具**由 **MCP Server** 提供；Agent 内 **Capability / ToolGraph / Planner 决策模型不变**，仅执行后端改为 **MCP**。

42. **MCP 执行后端**：包 **`mcpclient`**，**`Runner`**（单连接）与 **`MCPRouter`**（多 endpoint）提供 `ListCapabilities`（`list_tools`）与 `Invoke`（`CallTool`）；支持 `Connect` / `ConnectForApp(endpoint)`（HTTP 流式）。  
    **进度**：✅（`agent/pkg/mcpclient/runner.go`）

43. **工具清单与 Agent 同步**：`list_tools` 分页拉全量并缓存；MCP 模式直接以远端工具清单作为运行时能力来源。  
    **进度**：✅（`agent/pkg/mcpclient/runner.go` 清单缓存；**`toolsListCacheTTL`（1min）** 过期后在 `ListCapabilities` / `Run` 前重拉）

44. **运维与热插拔约定**：工具清单以 **1 分钟 TTL** 懒刷新（无 `ToolListChanged` 推送路径）。部署与约定见 **`docs/MCP.md`**。（Trace/审计在 MCP 调用上的专用打点仍可按需加强。）  
    **进度**：✅ 文档 + 刷新机制；🟡 可观测性深化

（原「泛化 ToolRuntime / 多 Source 组合」收敛为：**生产路径 = MCP**；测试在同模块 **`agent/`** 下可用 **`apptest.StubCapabilities`** 等注入 **`mcpclient.MCPRouter`** 兼容实现。）

45. **Capability / Tool 强类型 Schema**：`Runner.CachedTools()` 暴露 MCP 侧 **Name / Description / InputSchema**；`PlannerToolAppendix` 生成可拼进 prompt 的文本；**`NewDispatcher`** 写入 **`ToolAppendix` / `ToolInputSchemas`**。  
    **进度**：✅（附录与 schema 表已进 **`Dispatcher`**；其余深化按需）

46. **用户可见最终回复路径**：明确 Turn 末 `Reply` 来源——**工具结构化输出** 或 **独立合成步骤（如二次 LLM）**；避免长期依赖「trace Summary 拼接 + `finish`=done」。  
    **进度**：🟡

47. **ToolGraph + Skill → 工具组合产品化（远期）**：基于成功轨迹与 Skill 库，生成或推荐固定 Capability→Tool 链模板（半自动「工作流」），与 40–41 策略学习可协同。  
    **进度**：🟡 远期

48. **Ingress 任务队列**：在入口与 `RunInput` 之间缓冲、背压（与 **`Dispatcher.Run`** 内事件链正交）。  
    **进度**：❌ 未实现（形态待定）

---

## 开发优先级建议

### Learning Control（流水账 36–41）

| 优先级 | 流水账项 | 说明 | 状态 |
|--------|----------|------|------|
| **P1** | 36. Skill 生命周期管理 | hot/warm/cold/bad，过滤 Bad | ✅ |
| **P1** | 37. Trajectory Credit Assignment | 路径回溯加权更新 | ✅ |
| **P2** | 38. Policy Learner 落地 | DecideStrategy 数据通路 | ✅ |
| **P2** | 39. Exploration Decay | 探索衰减 | ✅ |
| P3 | 40. Strategy 级学习 | 各策略成功率与阈值 | 🟡 |
| 远期 | 41. 统一价值函数 | Value(task, strategy) | 🟡 |

### 下一阶段：**MCP 解耦工具层**（流水账 42–47）

| 优先级 | 流水账项 | 说明 | 状态 |
|--------|----------|------|------|
| **P0** | 42. MCP **`mcpclient.MCPRouter`** | `ConnectForApp` + `app.NewFromWorkspace`；测试 `apptest.NewFromLocalCapabilities` | ✅ |
| **P0** | 43. 清单与 capabilities 对齐 | 启动校验 + 缓存 | ✅ |
| **P0** | 44. 热插拔与运维约定 | `docs/MCP.md` + 清单 **1min TTL** 懒刷新 | ✅ / 🟡 |
| **P1** | 45. 强类型 Schema | `CachedTools` + `PlannerToolAppendix`；**`NewDispatcher`** 已写入 **`ToolAppendix`** | ✅ |
| **P2** | 46. 最终 Reply 路径 | 产品可用回答 | 🟡 |
| 远期 | 47. Graph+Skill 组合产品化 | 模板化工具链 | 🟡 |

### Ingress 任务队列（流水账 48）

| 优先级 | 流水账项 | 说明 | 状态 |
|--------|----------|------|------|
| **P1** | 48. Ingress 队列 | channel / worker / 按 session 串行等待定方案 | ❌ |
| P2 | 48 演进 | 多 worker、持久化 | 🟡 |

---

## 验收

```bash
cd OctoSuckerPlus && go test ./...
```

**使用说明**：运行方式、HTTP API、MCP 联调见 `docs/USAGE.md`。**Agent 官方入口** 使用 workspace **`config.json`**；**MCP 服务进程** 仍各自使用其环境变量（见 `mcpsvc/` 与 `.env.example`）。**演进路线图与落地状态**见本文 **「演进路线图」** 一节。

## 包/路径速查

| 路径 | 作用 |
|------|------|
| `agent/internal/runtime/app/` | 组装 **`engine.Dispatcher`**；**`RunInput` / `RunEvents`**、**`RerunSessionPlan`**、**`ErrRerunNoPlan`**、**`HTTPHandler`** |
| `agent/internal/config/` | `Workspace`（`config.json`）、`LoadCapabilities`（能力图 JSON → map） |
| `agent/cmd/` | `octoplushttp` |
| `agent/internal/` | `config`、`runtime/app`（含测试用 **`Dispatcher`** 构造）等（Go `internal` 边界） |
| `agent/pkg/telegram/` | Bot 入站 `RunPoll`、`Ingress`（不依赖 `mcpsvc/telegram`） |
| `agent/internal/runtime/store/`（`package store`） | **`SessionStore`**（`Get` / `Put`）、**`RoutingGraph`**、**`RecallCorpus`**、**`CapabilityRegistry`**、**`SkillRegistry`**（**`SkillEntry`**、**`CloneSkillPlan`**、**`BuildSkillEntryFromSession`**） |
| `agent/internal/runtime/engine/`（`package engine`） | **`Dispatcher`**（**`MaxSteps`**、**`Run`**、`dispatchEvent`）、`NewDispatcher`；**`dispatchUserInput` / `dispatchPlanProgress` / `dispatchToolCall` / `dispatchObservation` / `dispatchTrajectoryCheck` / `recordSkillLearning` / `archiveRecall`**；字段 **Sessions / RouteGraph / CapRegistry / Skills**、MCPRouter、Embedder、PlannerLLM、TrajectoryLLM 等；**`ports.Plan` 状态方法在 `pkg/ports`** |
| `agent/pkg/llmclient/` | OpenAI、Fixed |
| `agent/pkg/ports/` | 领域 DTO / 载荷、`Event`、**`Ev*`** / **`Payload*`**、`EventsFromJSON`、`EventsToJSON`、`Session.ExportSnapshot`、`ImportSnapshot`、`CloneSession`（**无接口**） |
| `agent/pkg/mcpclient/` | MCP Client（`package mcpclient`）、**`MCPRouter`** / **`Runner`**、`ConnectMCPRouter`、`list_tools` 缓存、`PlannerToolAppendix` |
| `mcpsvc/` | **stdio MCP Server**（`telegram/`、`web/` 等）；**`mcpsvc/registry`** 插件描述与 JSON 加载；`scripts/mcp-build.sh` |
