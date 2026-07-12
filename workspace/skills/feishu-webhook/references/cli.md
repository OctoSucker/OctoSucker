# feishu-send CLI

Install:

```bash
go install github.com/OctoSucker/tools-feishu/cmd/feishu-send@latest
```

Required environment:

```bash
export FEISHU_BOT_WEBHOOK_URL="https://open.feishu.cn/open-apis/bot/v2/hook/..."
export FEISHU_BOT_SECRET="..." # only when signature verification is enabled
```

Send plain text with `run_command`:

```json
{
  "program": "feishu-send",
  "args": ["text", "--message", "message"]
}
```

Send a prepared report file:

```json
{
  "program": "feishu-send",
  "args": ["text", "--file", "report.md"]
}
```

The executable must be visible to the OctoSucker process. Do not guess an absolute path or request webhook secrets from tool output.
