package agent

import (
	"context"
	"log"
	"time"
)

// cleanupSessions 清理过期会话
func (a *Agent) cleanupSessions(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute) // 每 5 分钟清理一次
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.sessionsMu.Lock()
			now := time.Now()
			for id, session := range a.sessions {
				if now.Sub(session.LastActiveAt) > a.maxSessionAge {
					delete(a.sessions, id)
					log.Printf("Cleaned up expired session: %s", id)
				}
			}
			a.sessionsMu.Unlock()
		}
	}
}
