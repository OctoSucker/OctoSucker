# OctoSucker Agent Instructions

This repository contains a Go ReAct-style agent runtime.

Runtime principles:

- Keep execution serial by default. Do not introduce parallel tool execution unless the user explicitly asks for it.
- Keep `agentloop`, `planning`, `execution`, `evaluation`, and `responding` domain-agnostic. Never add keyword intent routing or a named business workflow to those packages.
- Add executable capabilities through MCP or typed providers. Use Skills only for procedural guidance and structured-tool composition.
- Never make the model reconstruct a maintained CLI's executable name or argv from Skill prose.
- Preserve the append-only trajectory and synthesize user-facing answers from all useful observations, not only the last tool output.
- Treat tool/environment failures as actionable diagnostics. Never repeat the same failed tool and arguments in one turn.
- Learned routing is advisory. It must never bypass the planner or schema validation.
- Keep tool providers independent from runtime state by exchanging types through `internal/toolcontract`.
- Do not embed webhook secrets or API keys in source files.

Validation:

- Run `go test ./...` from the repository root after runtime changes.
- Run interaction smoke tests through the Task HTTP API or the Web UI.

Safety:

- Do not write API keys, Feishu webhooks, signing secrets, or private tokens into tracked files.
- Use environment variables for secrets.
- Surface missing environment variables clearly instead of letting the planner loop.
