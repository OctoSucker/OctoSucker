# us-market CLI

Install:

```bash
go install github.com/OctoSucker/tools-us-market/cmd/us-market@latest
```

Set a SEC-friendly user agent:

```bash
export US_MARKET_USER_AGENT="OctoSucker tools-us-market contact@example.com"
```

Recent filings for one ticker:

```json
{"program":"us-market","args":["filings","--ticker","NVDA","--forms","8-K,10-Q,10-K","--limit","10"]}
```

Multiple tickers:

```json
{"program":"us-market","args":["filings","--tickers","NVDA,AAPL,MSFT","--forms","8-K,10-Q,10-K","--limit","5"]}
```

Trading halts:

```json
{"program":"us-market","args":["halts","--limit","20"]}
```

Daily scan:

```json
{"program":"us-market","args":["scan","--tickers","NVDA,AAPL,MSFT","--forms","8-K,10-Q,10-K,13D,13G,S-1,424B5","--limit","5","--macro"]}
```
