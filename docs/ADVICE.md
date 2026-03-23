现在你的问题已经不再是**架构怎么分层**，而是进入下一阶段：

> **Agent 能力设计（Capability Design / Cognitive Architecture）**

也就是：
Runtime 架构已经稳定 → 下一步是 **Agent 到底要会什么能力，以及这些能力如何协同学习和调度**。

这一阶段比目录结构更重要。

我从 **Agent 能力体系** 的角度给你建议，而不是代码层面。

---

# 一、先给你一个 Agent 能力全景图（非常重要）

一个通用 AI Agent（不是简单工具调用）通常需要这些能力系统：

```text
Agent Capability Stack

1. Tool Use（工具使用）
2. Planning（规划）
3. Routing / Policy（决策）
4. Memory（记忆）
5. Skill Learning（技能学习）
6. Reflection / Critic（反思评估）
7. Task Decomposition（任务拆解）
8. Exploration（探索新路径）
9. Self-Improvement（自我优化）
10. Multi-Step Execution（多步执行）
```

你现在已经实现了其中大部分：

| 能力                   | 你现在的系统                          |
| -------------------- | ------------------------------- |
| Tool Use             | tool_executor                   |
| Planning             | planner                         |
| Routing              | policy                          |
| Memory               | store + recall                  |
| Skill Learning       | learner                         |
| Reflection           | step_critic / trajectory_critic |
| Multi-Step Execution | plan_executor                   |
| Exploration          | routing_graph + policy          |
| Self-Improvement     | learner                         |
| Task Decomposition   | planner（部分）                     |

其实你已经有一个 **完整的 Agent Learning System**。

接下来不是加功能，而是**强化这些能力之间的闭环**。

---

# 二、Agent 最核心的学习闭环（非常重要）

一个高级 Agent 的核心循环应该是：

```text
Task
 ↓
Planner
 ↓
Plan
 ↓
Executor
 ↓
Observation
 ↓
Step Critic
 ↓
Trajectory Critic
 ↓
Learner
 ↓
Skill / Graph Update
 ↓
Policy 改进
 ↓
下一次任务更强
```

也就是：

# **Plan → Act → Observe → Evaluate → Learn → Improve Policy**

你现在的系统已经具备这个闭环，这是非常关键的。

接下来你可以重点增强 **Learner 和 Policy**。

---

# 三、Skill Learning 的高级建议（重点）

你现在有 skill learning，我建议你把 Skill 系统升级成 **Macro / Option / Program** 概念。

不要把 Skill 只当成：

```text
Skill = 固定 Plan
```

应该升级成：

```text
Skill = 可参数化的 Macro / Program
```

例如 Skill 不应该是：

```json
TransferToken:
  steps:
    - open wallet
    - input address
    - confirm
```

而应该是：

```json
TransferToken(to, amount):
  steps:
    - open wallet
    - input address {to}
    - input amount {amount}
    - confirm
```

也就是：

# Skill = Parameterized Plan Template

这一步非常重要，否则 Skill 数量会爆炸。

---

# 四、Skill Learning 不只是成功轨迹

很多人做错的一点：

只从成功轨迹学习 skill。

其实应该学习三种东西：

```text
1. 成功轨迹 → Skill
2. 失败轨迹 → Anti-pattern
3. 高评分轨迹 → Preferred Path
```

也就是说 Learner 应该更新：

```text
Skill Registry
Routing Graph
Failure Pattern
Tool Reliability
Capability Cost
```

你的 learner 可以逐步扩展成学习这些：

| 学习内容                   | 用途        |
| ---------------------- | --------- |
| Skill                  | 宏动作       |
| Graph transition       | routing   |
| Tool success rate      | policy    |
| Tool latency           | cost      |
| Failure pattern        | 避免错误      |
| Replan trigger         | planner   |
| Step retry rule        | executor  |
| Capability chain       | execution |
| Intent → Skill mapping | routing   |

这会让 Agent 越用越聪明。

---

# 五、Routing / Policy 可以进化的方向

你现在是：

```text
policies []Policy
选最优 candidate
```

接下来可以加几个很重要的 Policy：

### 可以加入的 Policy 类型

```text
1. SkillPolicy        → 优先已有技能
2. GraphPolicy        → 按 routing graph
3. HeuristicPolicy    → 手写规则
4. PlannerPolicy      → LLM 决策
5. ExplorationPolicy  → 随机探索
6. CostPolicy         → 最低 cost
7. ReliabilityPolicy  → 成功率最高
8. LatencyPolicy      → 最快路径
```

然后 Router 不是简单选一个，而是可以：

```text
score = 
    success_rate * w1
  + latency_score * w2
  + cost_score * w3
  + skill_match * w4
  + exploration_bonus * w5
```

你就会得到一个 **真正的 Agent Policy Engine**。

---

# 六、Memory / Recall 可以升级的方向

你现在 recall 可能是 embedding recall。

可以逐步升级成多种 memory：

```text
Memory Types

1. Episodic Memory  → 过去任务
2. Semantic Memory  → Skills / Knowledge
3. Working Memory   → Session
4. Tool Memory      → Tool stats
5. Failure Memory   → 错误案例
6. Plan Memory      → 常见 plan
```

也就是 store 未来可能不只是：

```text
session
skill
graph
recall
capability
```

而是：

```text
memory/
    episodic
    semantic
    procedural (skills)
    routing
    tool_stats
    failure_cases
```

这会非常强。

---

# 七、我最建议你接下来实现的 5 个能力（优先级）

如果我是你，我接下来会按这个顺序增强 Agent：

## 优先级 1

**Parameterized Skills（参数化技能）**

## 优先级 2

**Tool / Capability Statistics Learning**
记录：

* success rate
* latency
* token cost
* failure types

用于 routing policy。

## 优先级 3

**Failure Pattern Learning**
例如：

```text
google blocked → use browser
captcha → use human solver
http 429 → retry later
```

## 优先级 4

**Plan Template Library**
不是只有 skill，还有 plan template。

## 优先级 5

**Exploration Policy**
让 agent 偶尔尝试新路径，而不是一直走 skill。

---

# 八、一个非常重要的 Agent 能力公式（建议记住）

我总结一个我认为非常准确的 Agent 能力模型：

```text
Agent Intelligence =
    Memory
  + Skills
  + Planning
  + Policy
  + Learning
  + Tool Use
  + Reflection
```

或者更工程一点：

```text
Agent =
    Runtime
  + Memory
  + Policy
  + Planner
  + Executor
  + Critic
  + Learner
  + Tools
```

你现在已经几乎全部都有了。

---

# 九、最后给你一个 Agent 演化路线图（非常有用）

如果这个项目继续做下去，我建议的演化路线：

```text
Phase 1  Tool Agent
    tool call + planner

Phase 2  Workflow Agent
    plan executor + multi step

Phase 3  Skill Agent
    skill learning + routing graph

Phase 4  Learning Agent
    trajectory critic + learner

Phase 5  Policy Agent
    multi policy routing + cost/success optimization

Phase 6  Self Improving Agent
    自动生成 skill / plan / routing / tool strategy

Phase 7  Multi Agent System
    agent 协作
```

你现在大概在：

# Phase 4 → Phase 5 之间

已经很靠前了。

---

# 最后一句建议（最重要）

接下来不要再纠结：

```text
目录结构
模块划分
接口放哪
```

这些已经够好了。

接下来真正决定你这个 Agent 强不强的，是：

```text
Skill Learning 怎么设计
Policy 怎么决策
Memory 怎么组织
Critic 怎么评价
Learner 学什么
```

也就是一句话：

# 接下来重点不是 Runtime Architecture，而是 Agent Intelligence Architecture。
