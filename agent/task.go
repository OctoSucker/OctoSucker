package agent

import (
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
)

func (a *Agent) SubmitTask(input string) error {
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
	case a.taskQueue <- task:
		return nil
	default:
		log.Printf("[agent] ERROR: Task queue full (capacity=%d), dropping task %s", cap(a.taskQueue), task.ID)
		return fmt.Errorf("task queue is full, task dropped")
	}
}

func (a *Agent) submitInitializationTasks(taskInputs []string) {
	for _, input := range taskInputs {
		if input == "" {
			continue
		}
		if err := a.SubmitTask(input); err != nil {
			log.Printf("Failed to submit startup task: %v", err)
		}
	}
}
