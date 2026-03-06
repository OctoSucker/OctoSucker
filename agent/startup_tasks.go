// Package agent 提供 AI Agent 的核心功能
package agent

import (
	"log"
)

// submitInitializationTasks 提交 Agent 启动时的初始化任务
func (a *Agent) submitInitializationTasks() {

	// 任务 1: 检查并修复未正确加载的 Skill
	task1Input := `这是 Agent 启动后的第一个初始化任务：检查并修复所有 Skill 的加载状态。

请执行以下步骤：
1. 使用 list_skills 工具查看所有已注册的 Skill 及其加载状态
2. 识别哪些 Skill 加载失败（status="failed"），查看它们的 init_error 信息
3. 使用 read_config_file 工具读取配置文件，检查是否有相关 Skill 的配置（如 Telegram 的 bot_token 等）
4. 分析失败原因：
   - 如果配置文件中已有配置但 Skill 加载失败，尝试使用 reload_skill 工具重新加载该 Skill
   - 如果配置文件中缺少必要的配置（如 bot_token、API key 等），使用 log_message 工具记录需要哪些配置信息
5. 如果成功修复了 Skill，使用 log_message 工具记录修复结果

目标：确保所有 Skill 都能正确加载，如果配置文件中已有配置，尝试自动修复加载失败的 Skill。`

	if err := a.SubmitTask(task1Input); err != nil {
		log.Printf("Failed to submit skill check task: %v", err)
	}

	// 任务 2: 通过可用 Skill 与用户建立联系
	task2Input := `这是 Agent 启动后的第二个初始化任务：与用户建立联系。

请执行以下步骤：
1. 使用 list_skills 工具查看所有 Skill，特别关注那些 status="loaded" 的 Skill
2. 检查这些已加载的 Skill 提供了哪些工具（查看工具列表）
3. 如果发现 Telegram Skill 已加载（github.com/OctoSucker/skill-telegram）：
   a. 使用 get_telegram_listening_status 检查监听状态（通常应该已经是 running，因为会自动启动）
   b. 如果状态是 stopped，使用 start_telegram_listening 工具启动监听
   c. 使用 send_telegram_message 向用户发送一条消息，内容可以是：
      "Agent 已启动并准备就绪。当前已加载 X 个 Skill，其中 Y 个正常工作。"
4. 如果找到其他可用的通信工具，也尝试向用户发送消息
5. 如果没有找到可用的通信工具，使用 log_message 工具记录：
   - 当前没有可用的通信渠道
   - 需要配置哪些 Skill（如 Telegram bot_token）才能与用户通信

注意：Telegram Skill 如果配置了 bot_token，会在初始化时自动启动监听，所以通常监听状态应该是 running。

目标：建立与用户的通信渠道，向用户发送启动通知，或者明确告知需要什么配置才能建立联系。`

	if err := a.SubmitTask(task2Input); err != nil {
		log.Printf("Failed to submit user contact task: %v", err)
	}
}
