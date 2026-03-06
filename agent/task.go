package agent

import (
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
)

// SubmitTask 提交任务到 Agent 的任务队列
// 可以由 Skill（如 Telegram Skill 收到消息时）或 Agent 内部（如初始化任务）调用
// Skill 可以在 RegisterSkill 时通过类型断言获取此方法
// 注意：input 应该包含完整的任务描述和所有必要信息（如消息来源、用户信息等）
func (a *Agent) SubmitTask(input string) error {
	if input == "" {
		return fmt.Errorf("task input is required")
	}

	// 生成任务 ID
	taskID := fmt.Sprintf("task_%s", uuid.New().String())

	// 创建任务
	task := &Task{
		ID:        taskID,
		Input:     input,
		CreatedAt: time.Now(),
	}

	log.Printf("SubmitTask: received task - id=%s, input=%s", taskID, input)

	// 提交任务到队列
	a.SubmitTaskToQueue(task)

	log.Printf("SubmitTask: task submitted successfully - id=%s", taskID)
	return nil
}

// SubmitTaskToQueue 提交任务到队列（内部方法）
func (a *Agent) SubmitTaskToQueue(task *Task) {
	select {
	case a.taskQueue <- task:
		log.Printf("Task queue: current length=%d", len(a.taskQueue))
		log.Printf("Task %s submitted to queue (input: %s)", task.ID, task.Input)
	default:
		log.Printf("[ERROR] Task queue is full (capacity=%d), dropping task %s", cap(a.taskQueue), task.ID)
	}
}
