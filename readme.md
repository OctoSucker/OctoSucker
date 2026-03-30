# OctoSucker

用 Go 实现的智能体运行时：通过 **OpenAI 兼容 API** 做规划与判卷，结合 **可学习的工具路由图**、**MCP 工具**、**技能（Skills）** 与 **SQLite** 持久化；可选通过 **Telegram Bot** 接收用户输入，命令在 **Docker** 或 **macOS Sandbox** 中执行。

## 核心设计与优势

OctoSucker 把「能力空间」显式建成图，再让 **结构化路由** 与 **LLM** 分工协作，并在真实执行反馈上持续更新边统计——这是和「单条 prompt 里塞一堆 tool definition」的本质区别。

- **可维护的工具图（Tool / Capability Graph）**  
  能力以节点、合法跳转以边组织；内置能力与 MCP 暴露的工具会进入同一张图，拓扑可随 MCP 能力集变化而同步。Planner 在图上做**受约束的选路**：下一步候选来自图结构，而不是无边界的全量 tool 盲选，长期更易调试与演进。

- **图规划 + LLM 规划，两条线拧成一股绳**  
  **图侧**给出「当前状态下可走哪些边、哪条通路更可信」的骨架；**LLM 侧**负责意图理解、计划细化与在图边界内的路径补全。结构化解空间减少胡编工具调用，语言模型专注在语义与步骤编排上。

- **图上的学习能力：好路越走越顺，坏边自动让位**  
  每次路由转移会写入边的 **成功/失败** 统计，并滚动 **成本、延迟** 等经验值；选路与加权时，高成功率边对应更低的等效权重，失败累积的边会被抬高权重、在排序里后移。另保留**近期意图转移**（相似意图下过去哪条边更稳），并带少量**探索项**，避免数据稀疏时过早锁死一条次优路。统计持久化到 SQLite，重启后仍延续「记忆」。

## 功能概览

- **事件驱动**：用户输入进入 Dispatcher，由 Planner（图 + LLM）选路与生成计划，Executor 执行步骤，Judge 做轨迹与步骤评判。
- **工具与能力**：内置执行（shell）、技能、Telegram 等；可通过 MCP 接入更多工具，并汇入路由图。
- **数据**：任务、**路由边统计**、召回等状态落在工作区内的 SQLite（由程序自动创建/打开）。

## 环境要求

- [Go](https://go.dev/dl/) **1.25+**（与 `go.mod` 一致）
- 可用的 **OpenAI API Key**（或兼容 OpenAI 协议的网关与模型）
- 若 `exec.backend` 为 `docker`：本机已安装 **Docker**，并按需构建/拉取配置中的沙箱镜像（默认名见下方示例配置）

## 快速开始

1. **准备工作区目录**（任意路径），其中需包含 `config.json`。

2. **生成配置**：复制仓库中的示例并改名：

   ```bash
   cp agent/workspace/config.example.json /你的/工作区/config.json
   ```

3. **编辑 `config.json`**：至少填写 `openai.api_key`（以及按需填写 `base_url`、`model`、`embedding_model`）。若使用 Telegram，填写 `telegram.bot_token` 等；若使用 MCP，在 `mcp_endpoint` 中填入端点列表。

4. **编译**

   ```bash
   go build -o octosucker ./agent
   ```

5. **运行**（`-workspace` 指向含 `config.json` 的目录）

   ```bash
   ./octosucker -workspace /你的/工作区
   ```

   或在仓库根目录直接运行源码：

   ```bash
   go run ./agent -workspace /你的/工作区
   ```

进程启动后若配置了有效的 Telegram Bot，会通过轮询处理消息；使用 `Ctrl+C` 优雅退出。

## 配置说明（摘要）

| 区块 | 作用 |
|------|------|
| `openai` | API 密钥、Base URL、对话模型、Embedding 模型 |
| `mcp_endpoint` | MCP 服务地址列表，用于扩展工具 |
| `exec` | 命令执行后端：`docker` 或 `macos_sandbox_exec`；超时、黑名单、容器/沙箱相关参数 |
| `http` | 配置中有 `listen` 字段；当前 `agent/main.go` 入口未启动 HTTP 服务 |
| `telegram` | Bot Token、默认会话、允许的 `chat_id` 白名单 |
| `skills_dir` | 技能文件目录（可相对工作区根路径；留空则使用仓库默认技能路径逻辑） |

完整字段与默认值以 `agent/internal/config/config.go` 与 `agent/workspace/config.example.json` 为准。

## 仓库布局（简要）

- `agent/`：主程序与内部引擎、模型、工具适配等
- `agent/workspace/`：示例配置与技能示例等，可作为工作区模板参考
