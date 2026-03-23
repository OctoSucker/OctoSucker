# MCP 服务（独立进程）

各子目录是一个 **独立的 MCP Server**：自己的包、`cmd/mcp-*` 入口。**`octoplushttp` 不拉起子进程**；与 Agent 联调时在 workspace 的 **`config.json`** 里配置 **`mcp.endpoint`**（streamable HTTP 基址，例：`http://127.0.0.1:8765/mcp`）。

| 目录 | 二进制 | 说明 |
|------|--------|------|
| `telegram/` | `mcp-telegram` | Telegram Bot API；`-listen :端口` 为 HTTP，无 `-listen` 为 stdio |
| `exec/` | `mcp-exec` | `run_command`（工作区白名单、超时、`rm`→`.trash`）；需 **`EXEC_WORKSPACE_DIRS`** |
| `web/` | （待实现） | 见 `web/README.md` |

共享工具（仅能被 `mcpsvc/...` 下的包引用）：`internal/mcpx`（`CallTool` 文本与 JSON 辅助）。

从源码一次拉起当前已实现的 MCP（**不编译**，`go run`；仓库根执行）：

```bash
./mcpsvc/run-all.sh
```

可选环境变量：`MCP_TELEGRAM_LISTEN`（默认 `:8765`）、`MCP_EXEC_LISTEN`（默认 `:8766`）；缺 `TELEGRAM_BOT_TOKEN` 会跳过 Telegram，缺 `EXEC_WORKSPACE_DIRS` 会跳过 exec。显式跳过：`MCP_RUN_ALL_SKIP_TELEGRAM=1`、`MCP_RUN_ALL_SKIP_EXEC=1`。若存在仓库根 **`.env`** 会自动 `source`（需自行 `set -a` 已写在脚本内）。

一键编译全部已注册的二进制：

```bash
./scripts/mcp-build.sh
```

输出目录：仓库根目录 `bin/`。
