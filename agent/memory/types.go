package memory

import "context"

const MaxMemoryTextLen = 512

type MemoryItem struct {
	ID        string    `json:"id"`
	TaskID    string    `json:"task_id"`
	Text      string    `json:"text"`
	Vector    []float32 `json:"vector"`
	CreatedAt int64     `json:"created_at"`
}

type VectorMemory interface {
	Add(ctx context.Context, taskID, text string) error
	Search(ctx context.Context, query string, topK int) ([]MemoryItem, error)
}
