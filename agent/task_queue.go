package agent

import (
	"context"
	"log"
)

// runTaskQueueProcessor 处理任务队列（主循环）
// 参考 OpenClaw 的设计：每个任务启动一个独立的 ReAct 循环
func (a *Agent) runTaskQueueProcessor(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			log.Printf("Task queue processor: context cancelled, exiting")
			return
		case task := <-a.taskQueue:
			// 为每个任务启动独立的 ReAct 循环
			go a.runReActLoop(ctx, task)
		}
	}
}
