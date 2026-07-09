# OctoSucker Agent Instructions

This repository contains a Go ReAct-style agent runtime.

Runtime principles:

- Keep execution serial by default. Do not introduce parallel tool execution unless the user explicitly asks for it.
- Prefer deterministic intent routing for obvious workflows before asking the LLM to choose tools.
- Preserve structured tool output and synthesize user-facing answers from accumulated evidence, not only the last step.
- Treat tool/environment failures as actionable diagnostics. Do not retry commands that are missing binaries, missing environment variables, or blocked by the sandbox.
- For Feishu group replies, use the external `feishu-gateway` -> `octosucker -message -` path. Do not embed webhook secrets in source files.
- For U.S. market intelligence, collect structured data first, run LLM analysis second, and only send concise high-value messages to Feishu.

Validation:

- Run `go test ./...` from the repository root after runtime changes.
- For CLI smoke tests, prefer one-shot mode:
  `octosucker -workspace /Users/zecrey/Desktop/OctoSucker/OctoSucker/workspace -message "列出当前可用技能"`

Safety:

- Do not write API keys, Feishu webhooks, signing secrets, or private tokens into tracked files.
- Use environment variables for secrets.
- Surface missing environment variables clearly instead of letting the planner loop.
