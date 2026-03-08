package agent

import (
	"context"
	"log"
	"time"
)

// systemPrompt Agent 的系统提示词
// 定义了 Agent 的行为准则和任务处理流程
const systemPrompt = `你是一个 AI Agent，正在处理任务。

重要指令：
1. **首先理解用户意图**：仔细分析用户消息，判断用户想要什么：
   - 如果是任务指令（如"帮我获取聊天信息"、"读取文件"等），必须先执行任务，获取结果，然后向用户回复结果
   - 如果是简单聊天（如"你好"、"hey"），可以友好回复
   - 如果是查询请求（如"查询消息"、"获取信息"），必须先查询，然后回复查询结果

2. **执行任务流程**：
   - 对于任务指令：先调用相关工具执行任务 → 获取结果 → 使用 send_telegram_message 向用户回复结果
   - 对于查询请求：先调用查询工具 → 获取数据 → 整理结果 → 使用 send_telegram_message 向用户回复查询结果
   - 对于简单聊天：直接使用 send_telegram_message 友好回复

3. **任务完成标准（非常重要）**：
   - **任务指令类**：必须完成用户要求的任务，获取结果，然后回复结果。不能只回复问候而不执行任务。
   - **查询请求类**：必须执行查询，获取数据，然后回复查询结果。
   - **简单聊天类**：发送友好回复后任务完成。
   - 只有在成功执行任务并回复结果后，任务才算完成。

4. **关键原则**：
   - 不要跳过任务执行步骤，直接回复问候
   - 如果用户要求执行任务，必须先执行任务，再回复结果
   - 不要重复执行相同的操作（如重复发送相同的消息）
   - 一旦任务的核心目标达成（执行任务 + 回复结果），立即返回最终答案，停止调用工具

请根据当前情况，选择合适的工具来执行。工具的定义和参数请参考可用的工具列表。`

// runReActLoop 运行 ReAct 循环（Reasoning -> Acting -> Observing）
// 这是核心的决策循环，持续运行直到任务完成
func (a *Agent) runReActLoop(ctx context.Context, task *Task) {
	if a.llmClient == nil {
		log.Printf("[ERROR] LLM client not initialized, skipping task %s", task.ID)
		return
	}

	// 获取或创建会话
	session := a.getOrCreateSession(task.ID)

	// ReAct 循环：持续推理-行动-观察，直到任务完成
	iterations := 0
	for iterations < a.maxReActIterations {
		// 每轮开始时检查 context 取消，便于超时或关闭时及时退出
		select {
		case <-ctx.Done():
			log.Printf("ReAct loop cancelled for task %s: %v", task.ID, ctx.Err())
			return
		default:
		}

		iterations++

		// 1. Reasoning: LLM 推理下一步行动
		result, err := a.reasonNextAction(ctx, session, task)
		if err != nil {
			log.Printf("ReAct loop [%d/%d] reasoning failed: %v", iterations, a.maxReActIterations, err)
			break
		}

		// 2. 检查是否完成（LLM 返回了最终答案，没有工具调用）
		if !result.HasToolCalls {
			if result.Content != "" {
				log.Printf("ReAct loop completed with final answer: %s", result.Content)
			}
			break
		}

		// 3. Acting: 执行工具调用
		// 注意：工具调用结果会通过 AddToolCall 添加到会话历史
		// OpenAI 格式要求：assistant message with tool calls -> tool messages
		// 这里我们简化处理，直接执行工具并将结果添加到历史

		for _, toolCall := range result.ToolCalls {
			toolResult, toolErr := a.executeToolCall(ctx, toolCall)

			// 4. Observing: 将工具执行结果添加到会话历史
			session.AddToolCall(toolCall, toolResult, toolErr)

			// 如果工具执行失败，继续循环让 LLM 处理错误
			if toolErr != nil {
				log.Printf("Tool %s execution failed: %v", toolCall.Name, toolErr)
			} else {
				// 对于某些关键操作，如果成功执行，提示 LLM 任务可能已完成
				// 这有助于 LLM 更快地判断任务完成状态
				if toolCall.Name == "send_telegram_message" {
					// 发送消息成功后，添加一个提示，告诉 LLM 如果这是回复用户消息的任务，可以结束了
					// 注意：我们不在历史中添加这个提示，而是在下次循环时通过系统消息或上下文来引导
					log.Printf("Tool send_telegram_message executed successfully, task may be complete")
				}
			}
		}

		// 更新会话活跃时间
		session.LastActiveAt = time.Now()
	}

	if iterations >= a.maxReActIterations {
		log.Printf("ReAct loop reached max iterations (%d) for task %s", a.maxReActIterations, task.ID)
	}
}
