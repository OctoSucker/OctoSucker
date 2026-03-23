# OctoSuckerPlus 目录图

（仅目录与模块边界；不含每个 `.go` 文件名。）

```
OctoSuckerPlus/
├── agent/                          # Agent 侧（与 mcpsvc/ 并列）
│   ├── cmd/                        # Agent 可执行入口
│   │   └── octoplushttp/
│   ├── workspace/                  # 示例：config.example.json、default_agent_capabilities.json
│   ├── pkg/                        # Agent 共享库：ports（DTO）、mcpclient、llmclient、telegram
│   └── internal/                   # 仅 agent/ 子树可 import（Go internal 规则）
│       ├── config/                 # Workspace JSON、LoadCapabilities、路径辅助
│       ├── tests/                  # apptest、testutil、testdata/
│       └── runtime/                # 运行时组装与引擎
│           ├── app/                # App、NewFromWorkspace、BuildAppWithConfig
│           ├── store/              # package store：SessionStore、RoutingGraph、RecallCorpus、CapabilityRegistry、SkillRegistry
│           └── engine/             # package engine：dispatcher、planner、dispatch、tool_executor、step_critic 等
├── docs/                           # 说明文档（含本文件）
├── mcpsvc/                         # 独立 MCP 服务实现（与 agent/ 并列）
│   ├── internal/
│   │   └── mcpx/
│   ├── registry/                   # 插件描述注册表 + JSON 加载
│   ├── telegram/
│   │   └── cmd/
│   │       └── mcp-telegram/
│   └── web/
├── scripts/
│   └── mcp-build.sh
├── bin/                            # 编译产物（通常不入库或本地生成）
├── go.mod
├── go.sum
├── README.md
├── .env.example
└── .gitignore
```

演进方向与落地状态见 **`docs/ARCHITECTURE_AND_PROGRESS.md`**（「演进路线图」一节）。**`internal/` 分层约定**见 **`docs/AGENT_INTERNAL_LAYERS.md`**。

## Import 路径速记

| 区域 | 示例 |
|------|------|
| Agent 组装 | `.../agent/internal/runtime/app` |
| Agent 配置 | `.../agent/internal/config`（`Workspace`、`LoadCapabilities`） |
| Agent HTTP | `.../agent/internal/runtime/app`（`HTTPHandler`、`NewFromWorkspace` 按 `cfg.http.listen` → `HTTPServer`） |
| Telegram 入站（Bot 长轮询） | `.../agent/pkg/telegram`（`RunPoll`、`Ingress`） |
| Workspace 示例 / 本地运行 | 仓库根 `workspace/`（`config.example.json` 等）；无 MCP 测试桩 **`agent/internal/tests/apptest`**；**`testdata/`** 在 **`agent/internal/tests/testdata/`** |
| 领域 DTO / 事件载荷 | `.../agent/pkg/ports`（`Session`、`Plan`、`Event`、`ToolResult` 等，**无接口**） |
| 存储（内存） | `internal/runtime/store`（**`SessionStore`**、**`RoutingGraph`**、**`RecallCorpus`**、**`CapabilityRegistry`**；`package store`） |
| 可插拔契约（与实现同包或邻包） | `mcpclient`（**`MCPRouter`**）、`internal/runtime/engine`（**`Dispatcher`** 等） |
| MCP 客户端 | `.../agent/pkg/mcpclient`（`package mcpclient`） |
| MCP 插件表 | `.../mcpsvc/registry` |
