# OctoSucker

OctoSucker is a serial AI agent runtime written in Go. It powers a personal
desktop assistant through an asynchronous Task API while keeping the reasoning
loop small and inspectable:

```text
Planner -> Executor -> Step Evaluator -> Planner or Responder
```

The planner chooses one action at a time. Every tool call, request for user
input, and direct model response becomes a user-visible Step. The runtime can
pause for structured input or approval and then resume the same Task.

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
