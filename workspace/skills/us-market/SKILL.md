---
name: us-market
description: Collect structured U.S. market intelligence through the external us-market CLI, including SEC filings, Nasdaq trading halts, and macro source links. Activate for U.S. market scans and filing checks.
compatibility: Requires the us-market executable and a SEC-compliant US_MARKET_USER_AGENT environment value.
allowed-tools: run_command
metadata:
  version: "1"
---

# US Market Intelligence

Use the external `us-market` CLI through `run_command` to collect source data. Read [references/cli.md](references/cli.md) before constructing the command.

## Analysis Contract

1. Treat collection as an intermediate step, not the final report.
2. Evaluate each result against the user's trading-intelligence goal.
3. Routine filing covers, stale items, and no-signal results are legitimate no-send outcomes.
4. A useful item must explain the event, why it matters, likely trade relevance, next trigger, and source.
5. Never send a raw filing list to Feishu.
