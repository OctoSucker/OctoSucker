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
			loopCtx := ctx
			var cancel context.CancelFunc
			if a.taskTimeout > 0 {
				loopCtx, cancel = context.WithTimeout(ctx, a.taskTimeout)
			}
			go func() {
				if cancel != nil {
					defer cancel()
				}
				a.runReActLoop(loopCtx, task)
			}()
		}
	}
}
