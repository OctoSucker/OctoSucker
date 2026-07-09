---
name: feishu-webhook
description: "Send text reports to a Feishu/Lark group through the external `feishu-send` webhook CLI."
tags: [feishu, lark, webhook, bot, notification, cli]
---

# Feishu Webhook

Use this skill when the user asks to send a message to a Feishu/Lark group, or when a scheduled workflow needs to push a report to a Feishu group.

This integration is intentionally external: OctoSucker should call the local `feishu-send` CLI through `run_command`, not implement Feishu Open Platform app code inside the agent repository. Feishu's official Go SDK is useful for app APIs, event subscriptions, and interactive cards; this skill only needs the custom bot webhook protocol.

## Prerequisites

Install the CLI on the machine that runs `run_command`:

```bash
go install github.com/OctoSucker/tools-feishu/cmd/feishu-send@latest
```

OctoSucker resolves the binary in this order: `FEISHU_SEND_BIN`, `PATH`, then `~/go/bin/feishu-send`.

Configure the bot webhook through an environment variable:

```bash
export FEISHU_BOT_WEBHOOK_URL="https://open.feishu.cn/open-apis/bot/v2/hook/..."
```

Or save it in the local user config:

```bash
feishu-send config set-webhook --webhook "$FEISHU_BOT_WEBHOOK_URL"
```

If the bot enables signature verification, also set `FEISHU_BOT_SECRET` or pass `--secret` when saving config.

If OctoSucker's exec backend is Docker, the Docker image must contain `feishu-send` and the webhook configuration. For a local machine CLI setup, prefer `macos_sandbox_exec` so the agent can call the host binary and read the host config.

## Send Plain Text

Use `run_command`:

```json
{
  "program": "feishu-send",
  "args": ["text", "--message", "message"]
}
```

Read a prepared report file:

```json
{
  "program": "feishu-send",
  "args": ["text", "--file", "report.md"]
}
```

## Daily Market Intelligence Convention

For scheduled market reports:

1. Generate the report as concise Markdown text.
2. Keep the Feishu message short enough for a group notification.
3. Include only high-value items: ticker, why it matters, next trigger, and source.
4. Send through `feishu-send text --message`.
5. Record sent item hashes in the market-intel workspace state before or after sending, so repeated runs do not duplicate messages.
