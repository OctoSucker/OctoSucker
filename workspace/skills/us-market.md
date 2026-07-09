---
name: us-market
description: "Collect structured U.S. market intelligence through the external `us-market` CLI: SEC filings, Nasdaq trading halts, and macro source links."
tags: [us-market, stocks, sec, filings, halts, macro, cli, market-intelligence]
---

# US Market Intelligence

Use this skill when the user asks for U.S. stock market intelligence, SEC filing checks, trading halts, or a daily market scan.

This integration is intentionally external: OctoSucker should call the local `us-market` CLI through `run_command`, then judge importance and synthesize a concise answer or Feishu message.

## Install

```bash
go install github.com/OctoSucker/tools-us-market/cmd/us-market@latest
```

The binary is usually at `~/go/bin/us-market`.

Set a SEC-friendly user agent in the process environment when possible:

```bash
export US_MARKET_USER_AGENT="OctoSucker tools-us-market contact@example.com"
```

## Commands

Recent SEC filings for a ticker:

```json
{
  "program": "us-market",
  "args": ["filings", "--ticker", "NVDA", "--forms", "8-K,10-Q,10-K", "--limit", "10"]
}
```

Multiple tickers:

```json
{
  "program": "us-market",
  "args": ["filings", "--tickers", "NVDA,AAPL,MSFT", "--forms", "8-K,10-Q,10-K", "--limit", "5"]
}
```

Trading halts:

```json
{
  "program": "us-market",
  "args": ["halts", "--limit", "20"]
}
```

Daily scan:

```json
{
  "program": "us-market",
  "args": ["scan", "--tickers", "NVDA,AAPL,MSFT", "--forms", "8-K,10-Q,10-K,13D,13G,S-1,424B5", "--limit", "5", "--macro"]
}
```

Feishu-ready report:

```json
{
  "program": "/bin/zsh",
  "args": [
    "-lc",
    "us-market report --tickers NVDA,AAPL,MSFT --forms 8-K,10-Q,10-K --limit 3 | feishu-send text --stdin"
  ]
}
```

## Agent Convention

For daily market reports:

1. Use `us-market scan` to collect source JSON.
2. Analyze that JSON with OctoSucker's `analyze_us_market_intel` LLM tool.
3. Send only the LLM-approved concise message through `feishu-send`.
4. Do not push raw filing lists to Feishu. Every sent item should include event, why it matters, trade relevance, next trigger, and source.
