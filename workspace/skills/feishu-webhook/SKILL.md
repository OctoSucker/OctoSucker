---
name: feishu-webhook
description: Send concise text reports to a Feishu/Lark group through the external feishu-send webhook CLI. Activate for Feishu group notifications and scheduled report delivery.
compatibility: Requires feishu-send on the agent process PATH and FEISHU_BOT_WEBHOOK_URL; signed bots also require FEISHU_BOT_SECRET.
allowed-tools: run_command
metadata:
  version: "1"
---

# Feishu Webhook

Use this skill when the user asks to send a message to a Feishu/Lark group or a scheduled workflow must deliver a concise report.

This integration is external. Use `feishu-send` through `run_command`; do not implement Feishu application APIs inside OctoSucker.

Before sending, ensure the message content is known. Sending is an external side effect, so do not claim success until the command observation confirms it.

For exact installation, environment, and invocation details, read [references/cli.md](references/cli.md).

## Report Convention

1. Synthesize concise Markdown text before sending.
2. Include only decision-useful items, with the event, why it matters, next trigger, and source.
3. Do not push raw filing lists or unfiltered tool output to the group.
4. Preserve a legitimate no-send result when no item passes the user's value threshold.
