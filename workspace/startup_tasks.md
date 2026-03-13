这是 Agent 启动后的第一个初始化任务：检查并修复所有 Tool provider 的加载状态。

请执行以下步骤：
1. 使用 list_tool_providers 工具查看所有已注册的 Tool provider 及其加载状态
2. 识别哪些加载失败（status="failed"），查看 init_error 信息
3. 使用 read_config_file 读取配置文件，检查是否有相关配置（如 Telegram 的 bot_token 等）
4. 分析失败原因：
   - 若配置文件中已有配置但加载失败，尝试使用 reload_tool_provider 重新加载该 provider
   - 若缺少必要配置（如 bot_token、API key 等），使用 log_message 记录需要哪些配置
5. 若成功修复，使用 log_message 记录修复结果

目标：确保所有 Tool provider 正确加载，若配置已有则尝试自动修复加载失败的 provider。

---

这是 Agent 启动后的第二个初始化任务：启动 Telegram 监听。

请执行以下步骤：
1. 使用 list_tool_providers 查看已加载的 provider。
2. 若 Telegram 相关 provider 已加载：
   a. 调用 get_telegram_listening_status；若为 stopped，调用 start_telegram_listening 启动监听。
3. 若无可用通信工具，用 log_message 记录需要配置的 provider。

目标：确保 Telegram 已正确启动监听，Agent 能够被动接收消息。

---

这是 Agent 启动后的第三个初始化任务：向用户发送欢迎信息与 Tool provider 列表。

请执行以下步骤：
1. 使用 list_tool_providers 获取所有已加载的 provider 及其状态。
2. 若 Telegram 相关 provider 已加载：
   a. 调用 get_telegram_allowed_chat_ids 获取允许的 chat_id/user_id 列表。
   b. send_telegram_message 的 chat_id 必须且只能使用该工具返回的 allowed_chat_ids 或 allowed_user_ids 中的某一个数字，禁止使用任何未在该列表中的 ID（禁止编造、猜测或使用示例数字如 123456789）。
   c. 构造一条欢迎消息，其中包含：
      - Agent 已启动并准备就绪的说明
      - 当前已加载的 Tool provider 列表（来自 list_tool_providers 的结果概要）
   d. 使用 get_telegram_allowed_chat_ids 返回的任一 ID 作为 chat_id，调用 send_telegram_message 发送上述欢迎消息。
3. 若无可用通信工具，用 log_message 记录需要配置的 provider。

目标：仅使用 get_telegram_allowed_chat_ids 返回的 ID 发送欢迎信息与 provider 列表，或记录无可用渠道。

---

这是 Agent 启动后的第四个初始化任务：汇报 MCP 能力。

请执行以下步骤：
1. 将 mcp_list_servers 的返回结果进行简要整理，总结出：
   - 每个 MCP server 的 id / url / 描述
   - 每个 server 下可用的工具名称及简要用途
2. 使用 send_telegram_message，将上述 MCP 能力概要发送给用户，让用户清楚当前 Agent 拥有哪些外部服务与工具。
3. 若 tools-mcp 未加载或 mcp_list_servers 调用失败，用 log_message 记录具体原因，必要时通过 send_telegram_message 告知用户当前暂不可用 MCP 能力。

目标：在 Agent 启动时，向用户清晰汇报当前已接入的所有 MCP 服务及其提供的工具能力。
