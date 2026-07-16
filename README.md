# OctoSucker

OctoSucker is a serial AI agent runtime written in Go. Its core is a bounded
action loop, not a collection of hard-coded workflows:

```text
Planner -> Executor -> Evaluator -> Planner or Responder
```

The runtime combines builtin tools, MCP servers, and typed executable providers
in one validated tool catalog. Directory-based Agent Skills add procedural
instructions through explicit activation. Domain behavior belongs in those
extensions; the core loop does not know about individual websites, markets, or
messaging services.

## Architecture

| Path | Responsibility |
| --- | --- |
| `cmd/octosucker` | Process flags, config, runtime, and gateway wiring |
| `config` | Workspace configuration and path resolution |
| `internal/runtime/agentloop` | Generic serial turn controller and limits |
| `internal/runtime/model` | Append-only turn, action, observation, and assessment state |
| `internal/runtime/contextmanager` | Role-specific token budgets, selection, and trajectory compaction |
| `internal/runtime/planning` | One structured action-or-respond decision |
| `internal/runtime/execution` | Tool invocation and result normalization |
| `internal/runtime/evaluation` | Goal progress and semantic action evaluation |
| `internal/runtime/responding` | Final user-facing synthesis |
| `internal/runtime/conversation` | Bounded in-memory context by conversation id |
| `internal/runtime/toolrouting` | Conservative learned routing recommendations |
| `internal/skills` | Agent Skill discovery, validation, and resource access |
| `internal/toolcontract` | Tool DTOs and full JSON Schema validation |
| `internal/tools` | Registry, builtin providers, MCP sessions, and typed executable adapters |
| `internal/tools/opencli` | OpenCLI help introspection, generated schemas, and deterministic argv compilation |
| `internal/storage` | Workspace SQLite persistence |
| `internal/gateway` | Ingress assembly |
| `internal/ingress` | Telegram and admin HTTP adapters |

The detailed contracts are documented in
[`internal/runtime/DESIGN.md`](internal/runtime/DESIGN.md).

## Runtime Properties

- One action is planned and executed at a time.
- The planner receives exact tool ids, descriptions, risk metadata, and schemas.
- Tool arguments are validated against full JSON Schema before invocation and
  again at the registry boundary.
- Structured model decisions use JSON response mode with validation retries.
- Planner, evaluator, and responder use independently configured models.
- Prompt context is budgeted by role; old steps are compacted as structured summaries rather than cut mid-output.
- The execution trajectory is append-only; failed attempts remain visible.
- Exact failed actions cannot be repeated in the same turn.
- Tool success and user-goal success are evaluated separately.
- Goal progress and routing-learning value are independent; useful prerequisite
  actions can teach routing without prematurely completing the turn.
- Final answers synthesize all useful observations.
- Learned routing is advisory and never bypasses the planner.
- Conversation context is separated by ingress-supplied conversation id.
- Activated Skill instructions persist in the conversation independently of
  bounded execution observations.
- Observation provenance prevents untrusted tool output from becoming agent
  instructions or leaking unrelated secrets.

## Workspace

`-workspace` points to an existing agent home containing resources such as:

- `config.json`
- `skills/`
- `knowledge/`
- `data/`
- `logs/`

Each Skill is a directory with a required `SKILL.md` and optional supporting
resources:

```text
skills/opencli/
|-- SKILL.md
`-- references/
    `-- authentication.md
```

Only Skill name and description enter the default catalog. The complete
`SKILL.md` is injected after `activate_skill`; supporting resources are read on
demand with `read_skill_resource`.

Secrets remain in local configuration or environment variables. Do not commit
`workspace/config.json`, SQLite data, or logs.

## Run

```bash
go build -o octosucker ./cmd/octosucker
./octosucker -workspace /path/to/workspace
```

The admin HTTP endpoint accepts an optional conversation id for integrations
that still use the synchronous chat endpoint:

```json
{
  "conversation_id": "browser-main",
  "message": "继续刚才的分析"
}
```

If omitted, the local admin conversation id is `admin`.

## Extension Model

Use one of these boundaries for new capabilities:

1. MCP provider for a structured service.
2. A typed provider that deterministically adapts an executable into
   schema-validated tools.
3. A directory-based Agent Skill for procedural knowledge and workflow rules.

Do not use a Skill document as an executable protocol. Stable integrations must
not require the model to reconstruct a binary name, flags, or argv from prose.
Keep `run_command` for genuinely ad hoc commands rather than as the interface to
a maintained capability.

The optional OpenCLI provider demonstrates the typed executable boundary. At
startup it reads `opencli <site> --help -f yaml`, exposes only the configured
allowlist, and always compiles structured arguments to JSON-producing commands.
Configure it under `opencli.command`, `opencli.command_timeout_sec`, and
`opencli.commands` in `config.json`.

Do not add domain intent matching or domain-specific state transitions to the
runtime loop. Stable business workflows should live in the external tool or be
described by a Skill that composes already-structured tools.

## Validation

```bash
go test ./...
go vet ./...
```
