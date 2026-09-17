package runtime

import (
	"strings"
	"sync"
)

const defaultMaxConcurrentTurns = 4

// turnGate keeps one active turn per conversation/task while allowing a small,
// bounded number of independent turns to make progress concurrently.
type turnGate struct {
	mu     sync.Mutex
	active map[string]struct{}
	slots  chan struct{}
}

func newTurnGate(limit int) *turnGate {
	if limit <= 0 {
		limit = defaultMaxConcurrentTurns
	}
	return &turnGate{active: make(map[string]struct{}), slots: make(chan struct{}, limit)}
}

func (g *turnGate) tryAcquire(key string) (func(), bool) {
	if g == nil {
		return func() {}, true
	}
	key = strings.TrimSpace(key)
	if key == "" {
		key = "default"
	}
	select {
	case g.slots <- struct{}{}:
	default:
		return nil, false
	}

	g.mu.Lock()
	if _, exists := g.active[key]; exists {
		g.mu.Unlock()
		<-g.slots
		return nil, false
	}
	g.active[key] = struct{}{}
	g.mu.Unlock()

	var once sync.Once
	release := func() {
		once.Do(func() {
			g.mu.Lock()
			delete(g.active, key)
			g.mu.Unlock()
			<-g.slots
		})
	}
	return release, true
}
