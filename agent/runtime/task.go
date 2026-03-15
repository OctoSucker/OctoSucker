package runtime

import (
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
)

type Task struct {
	ID        string
	Input     string
	CreatedAt time.Time
}

func (r *AgentRuntime) SubmitTask(input string) error {
	if input == "" {
		return fmt.Errorf("task input is required")
	}
	task := &Task{
		ID:        fmt.Sprintf("task_%s", uuid.New().String()),
		Input:     input,
		CreatedAt: time.Now(),
	}
	inputPreview := task.Input
	if len(inputPreview) > 80 {
		inputPreview = inputPreview[:80] + "..."
	}
	select {
	case r.taskQueue <- task:
		return nil
	default:
		log.Printf("[agent] ERROR: Task queue full (capacity=%d), dropping task %s", cap(r.taskQueue), task.ID)
		return fmt.Errorf("task queue is full, task dropped")
	}
}
