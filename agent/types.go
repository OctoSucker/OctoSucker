package agent

import "time"

type Task struct {
	ID        string
	Input     string
	CreatedAt time.Time
}
