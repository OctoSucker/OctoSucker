# OctoSucker

OctoSucker is a learning project for exploring how a practical personal AI
assistant should be designed. It is not an attempt to reproduce every feature
of a coding agent. The project focuses on a smaller question: how can an agent
remain understandable, extensible, and useful to ordinary desktop users while
it performs real work?

The current answer is a serial Go runtime exposed through an asynchronous Task
API, with a deliberately small and inspectable reasoning loop:

```text
Planner -> Executor -> Step Evaluator -> Planner or Responder
```

The planner chooses one action at a time. Every tool call, request for user
input, and direct model response becomes a user-visible Step. The runtime can
pause for structured input or approval and then resume the same Task.

## Design Principles

### One assistant, many Tasks

The user should experience one persistent digital assistant rather than a list
of unrelated chat sessions. Internally, work is still divided into Tasks because
execution needs clear lifecycle, failure, approval, and audit boundaries. A Task
is therefore an execution unit, not the product's primary mental model.

When a Task needs more information, the next input continues that Task. After a
Task finishes, the next request creates a child Task while preserving assistant
context. This keeps the interface continuous without pretending that unrelated
work belongs to one execution.

### Plan one step from current evidence

OctoSucker does not ask the model to invent a complete execution graph before
work begins. For open-ended office tasks, later actions often depend on tool
results or information that the user has not supplied yet. A large up-front
plan can look convincing while being based on assumptions.

The planner instead chooses one next action from the current goal, context, and
observations. After execution, an evaluator judges whether the result moved the
goal forward, and the planner decides again:

```text
Goal + Context
      |
      v
Plan one action -> Execute -> Evaluate usefulness and progress
      ^                            |
      |____________________________|
                    |
                    v
          Respond or request input
```

This is intentionally serial. Parallel execution may improve throughput, but it
also increases coordination cost and makes early behavior harder to understand.
For a personal assistant still being validated, debuggability and predictable
state transitions are more valuable than speculative concurrency.

### Separate execution facts from goal completion

A successful tool call only proves that the tool completed its contract. It
does not prove that the user's goal has been achieved. OctoSucker treats the
structured result returned by a builtin tool, MCP server, or typed provider as
the execution fact; the evaluator then judges that result against the Step goal
and the overall user goal.

This avoids two opposite errors: declaring success merely because a command
exited successfully, and repeatedly rechecking trusted tool results in an
endless chain of verification. Tool correctness belongs at the tool boundary;
goal-level judgment belongs in the agent loop.

### Make reasoning inspectable without exposing hidden chain of thought

The interface should show what the agent is doing, not private model reasoning.
Every tool action, information request, and direct response becomes a concise
Step with a title, status, summary, and evaluation. Failed attempts remain in
the trajectory so later decisions and debugging are based on the actual run.

The user sees operational progress and decisions rather than raw prompts or
hidden reasoning traces.

### Use structured interaction when the information is structured

An empty chat box places the entire burden of describing a task on the user.
That works well for people who already understand prompts, tools, and file
paths, but it raises the barrier for ordinary office users.

Natural language remains the entry point for open intent. Once the agent knows
which specific fields are missing, it can return a structured form with labels,
choices, validation, and defaults. High-risk actions similarly become explicit
approval controls inside the normal conversation flow. The UI adapts to the
current Task instead of forcing every interaction through prose.

### Keep the runtime generic and capability boundaries deterministic

The core loop should not contain special cases for individual websites,
markets, messaging products, or office workflows. New capabilities enter
through explicit boundaries:

- MCP for structured external services.
- Typed executable providers for deterministic command construction.
- Agent Skills for procedural knowledge and workflow guidance.

A Skill document can teach the model when and why to use a capability, but it
should not force the model to reconstruct an executable protocol from prose.
Stable commands, arguments, validation, and result shapes belong in MCP or a
typed provider.

### Learn conservatively

Past execution can improve future tool routing, but learned recommendations are
advisory. They may help the planner choose among capabilities; they must not
bypass planning, schema validation, risk policy, or evaluation. This keeps
learning useful without turning historical noise into hidden control flow.

### Prefer explicit product boundaries over premature infrastructure

OctoSucker currently favors a local, user-owned runtime, serial execution,
low-frequency HTTP polling, and in-memory Task state. These are deliberate
tradeoffs for a single-user learning project. Persistence, parallel execution,
subagents, and streaming transports should be added when observed usage proves
their value, not because mature agent products happen to contain them.

## Current Capabilities

- Serial, one-step planning with bounded retries and execution limits.
- Separate planner, evaluator, and responder model configuration.
- Role-specific context budgets and structured trajectory compaction.
- Builtin, MCP, and typed executable tools in one schema-validated catalog.
- Directory-based Agent Skills with explicit activation and on-demand resources.
- Asynchronous Tasks with messages, Steps, status, result, and error state.
- Structured forms when the agent needs missing information.
- Explicit approval before high-risk tool execution.
- Learned tool-routing recommendations without bypassing the planner.
- Local HTTP and optional Telegram ingress.

Task state is currently in memory. Restarting the process clears Tasks, but
workspace SQLite data, learned routing data, Skills, knowledge, and logs remain
on disk.

## Architecture

| Path | Responsibility |
| --- | --- |
| `cmd/octosucker` | Service entrypoint and lifecycle |
| `config` | Workspace configuration and path resolution |
| `internal/runtime/agentloop` | Serial turn controller and execution limits |
| `internal/runtime/model` | Append-only turn, Step, observation, and assessment state |
| `internal/runtime/contextmanager` | Role-specific context selection and compaction |
| `internal/runtime/planning` | Structured action-or-respond decisions |
| `internal/runtime/execution` | Tool invocation, approval, and result normalization |
| `internal/runtime/evaluation` | Step usefulness and goal-progress evaluation |
| `internal/runtime/responding` | Final user-facing synthesis |
| `internal/runtime/conversation` | Bounded assistant context |
| `internal/runtime/toolrouting` | Conservative learned routing recommendations |
| `internal/task` | In-memory Task lifecycle and snapshots |
| `internal/interaction` | Structured user-input forms |
| `internal/skills` | Skill discovery, validation, activation, and resources |
| `internal/toolcontract` | Tool DTOs, policies, and JSON Schema validation |
| `internal/tools` | Builtin, MCP, OpenCLI, and executable providers |
| `internal/storage` | Workspace SQLite persistence |
| `internal/gateway` | HTTP and Telegram ingress assembly |
| `internal/ingress/adminhttp` | Task, compatibility chat, and graph HTTP routes |

See [`internal/runtime/DESIGN.md`](internal/runtime/DESIGN.md) for the runtime
contracts and loop invariants.

## Workspace

The service requires a workspace directory containing `config.json` and its
agent-owned resources:

```text
workspace/
|-- config.json
|-- skills/
|-- knowledge/
|-- data/
`-- logs/
```

Start from the checked-in example:

```bash
cp workspace/config.example.json workspace/config.json
```

At minimum, configure the OpenAI-compatible endpoint and the planner,
evaluator, and responder models. To expose the desktop API, keep a local HTTP
listener enabled:

```json
{
  "http": {
    "listen": "127.0.0.1:8090"
  }
}
```

Do not commit real API keys, `workspace/config.json`, SQLite files, or logs.

## Run

```bash
go build -o octosucker ./cmd/octosucker
./octosucker -workspace ./workspace
```

The process is a service. There is no terminal chat mode. With the example HTTP
configuration, the API is available at `http://127.0.0.1:8090` and logs are
written to `workspace/logs/agent.log`.

## Task API

Create a Task by submitting assistant input:

```http
POST /api/assistant/input
Content-Type: application/json

{
  "content": "整理这批会议记录并生成摘要"
}
```

The server returns `202 Accepted` with an initial Task snapshot:

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

Read the latest snapshot with a low-frequency poll:

```http
GET /api/tasks/{taskID}
```

Task status is one of `running`, `waiting_input`, `waiting_approval`,
`completed`, `failed`, or `cancelled`.

When `pending_interaction` is present, submit its form values to the same Task:

```http
POST /api/tasks/{taskID}/interactions/{interactionID}
Content-Type: application/json

{
  "values": {
    "audience": "全体员工"
  }
}
```

Free-form input can also continue a Task that is in `waiting_input`:

```json
{
  "active_task_id": "...",
  "content": "通知对象是全体员工"
}
```

If `active_task_id` identifies a terminal Task, the input creates a new child
Task. Input is rejected while the active Task is still running or waiting for
approval.

Resolve a pending high-risk action explicitly:

```http
POST /api/tasks/{taskID}/approvals/{approvalID}
Content-Type: application/json

{
  "decision": "approved"
}
```

Use `rejected` to deny it. The runtime resumes the suspended execution after
either decision.

`POST /api/chat` remains available for synchronous integrations such as
Telegram-style adapters. The desktop frontend should use the Task API because
it exposes progress, interactions, and approvals.

## Extension Model

Use one of these boundaries for new capabilities:

1. MCP for a structured external service.
2. A typed provider that adapts an executable into schema-validated tools.
3. A directory-based Agent Skill for procedural knowledge and workflow rules.

A Skill is not an executable protocol. Stable integrations should not require
the model to reconstruct a binary name, flags, or argv from prose. Keep
`run_command` for genuinely ad hoc work.

Each Skill is a directory with a required `SKILL.md` and optional resources:

```text
skills/opencli/
|-- SKILL.md
`-- references/
    `-- authentication.md
```

Only the Skill name and description enter the default catalog. Full
instructions are injected after activation, and supporting resources are read
on demand.

## Validation

```bash
go test ./...
go vet ./...
```
