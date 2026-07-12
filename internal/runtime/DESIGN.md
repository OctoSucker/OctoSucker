# Runtime Design

## Purpose

`internal/runtime` is a serial, tool-using agent loop. It is not a workflow
engine and contains no domain-specific workflows.

One turn follows this control flow:

```text
conversation context + user goal
              |
              v
          Planner
       /             \
   respond          one Action
      |                 |
      |             Executor
      |                 |
      |            Observation
      |                 |
      |             Evaluator
      |        /          |          \
      |    continue    complete     blocked
      |        |          |          |
      |        +----------+----------+
      |                   |
      +--------------> Responder
                          |
                    user-facing answer
```

The planner is called again after `continue`. This is the replan operation;
there is no separate replan event and no trajectory truncation.

## Module Boundaries

### `runtime`

The composition root. It builds the registry, planner, executor, evaluator,
responder, optional routing learner, and conversation store. It owns the
single-flight turn lock and workspace resources.

### `agentloop`

The control policy for one turn. It only depends on narrow interfaces:

- `Planner`
- `Executor`
- `Evaluator`
- `Responder`
- optional `Learner`

It enforces action and consecutive-failure limits. It does not know any tool,
provider, CLI, skill, or business domain by name.

### `model`

The turn aggregate and its value types:

- `Turn`
- `Decision`
- `Action`
- `Observation`
- `Assessment`
- `Step`

`Turn.Steps` is an append-only execution trajectory. A failed or irrelevant
action remains visible to later decisions and final synthesis. Exact failed
actions are identified by a structured tool-and-arguments fingerprint.

`Turn.ContextArtifacts` is separate from the trajectory. Trusted tools may
produce durable context, such as an activated Agent Skill. These artifacts are
bounded, deduplicated, retained in process-local conversation state, and
injected into planning, evaluation, and response prompts without passing
through observation truncation.

Every observation also carries `output_trust`. Workspace instructions and
runtime metadata may guide the agent; ordinary CLI, MCP, browser, and external
content is treated as untrusted data and cannot supply new instructions.

### `contextmanager`

Builds role-specific prompt snapshots for planning, evaluation, and response.
It owns token estimation, section budgets, conversation selection, active
instruction selection, tool-catalog reduction, and trajectory compaction.

The latest steps are retained as complete structured units. When a trajectory
exceeds its budget, older steps become explicit summaries instead of being
silently cut in the middle of an observation. Context usage and omissions are
recorded in the turn trace. The context manager does not call an LLM or make
control-flow decisions.

### `planning`

Makes one structured decision:

- `act`, including an exact tool id and complete arguments
- `respond`, when tools are unnecessary, evidence is sufficient, clarification
  is required, or no legitimate action remains

Responses are classified as `answer`, `clarify`, or `blocked`, so a request for
missing information and an environmental failure are not recorded as successful
task completion.

The planner receives the complete tool catalog with JSON schemas. Selection and
argument generation happen in one model call, followed by full JSON Schema
validation and one corrective retry. Historical routing recommendations are
weak hints only and can never bypass the planner.

The registry snapshots tool descriptors and schemas during startup. Planning,
validation, and response grounding read this local snapshot; MCP network calls
occur only during explicit tool invocation.

### `execution`

Invokes one validated action and normalizes the result. It records policy
metadata but does not retry, evaluate progress, or produce user output.

### `evaluation`

Judges the latest successful observation against the whole user goal:

- `continue`
- `complete`
- `blocked`

It separately labels the latest action as `helpful`, `wrong_route`, or
`no_signal` for routing learning and records a bounded reason code. Progress and
routing value are independent: a necessary prerequisite can be useful learning
while the turn still needs to continue. Tool execution success alone never
implies goal success.

Technical tool failures do not require an LLM judgment. `agentloop` classifies
them generically and returns recoverable failures to the planner.

A successful typed tool result is the authoritative observation of that tool's
effect. The evaluator does not demand recursive read-back verification unless
the result itself is ambiguous, partial, pending, or independent verification
was explicitly requested.

### `responding`

Owns all user-facing synthesis. It sees conversation context and the bounded
full trajectory, so multi-step answers never degrade to the last tool output.

### `conversation`

Keeps bounded process-local dialogue history by an ingress-supplied conversation
id. It also retains trusted active context artifacts for the conversation.
HTTP, stdin, and each Telegram chat use separate ids. Persistence is outside the
current scope.

### `skills`

Owns directory-based Agent Skill discovery and validation. A Skill consists of
`<name>/SKILL.md` plus optional supporting files. The catalog enforces exact
names, required descriptions, bounded complete instructions, safe relative
resource access, UTF-8 input, and non-symlink files.

The tools layer exposes `activate_skill` and `read_skill_resource`; Skill files
never register executable providers or define process argv.

### `learning` and `toolrouting`

Semantic action outcomes are stored in SQLite. Recommendations require repeated,
similar successful examples. They are advisory prompt context, not an alternate
execution path. Learning persistence failures never fail the user turn.

### `toolcontract`

Shared tool result, policy, descriptor, and JSON Schema validation types. Tool
providers depend on this package rather than importing the agent runtime.

## Dependency Direction

```text
ingress -> gateway -> runtime composition
                         |
                         v
        planning / execution / evaluation / responding
                         |
                         v
                  model + toolcontract

tools/providers -----------------------> toolcontract
```

Tool providers must not import runtime state. Runtime modules must not inspect
specific business programs or tool output shapes.

## Extension Rules

- Add business capabilities as MCP tools or typed executable providers.
- Use Skills for procedural guidance and composition, never as a substitute for
  a structured execution contract.
- Put stable multi-step business orchestration in the external tool or a
  declarative skill, not in `agentloop`, `planning`, or `evaluation`.
- Add generic safety and lifecycle behavior to the loop only when it applies to
  every tool.
- Keep execution serial until parallel action semantics are explicitly needed.
