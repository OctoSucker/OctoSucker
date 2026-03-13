package memory

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/OctoSucker/octosucker/agent/llm"
	"github.com/google/uuid"
)

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

type noopVectorMemory struct{}

func (noopVectorMemory) Add(_ context.Context, _ string, _ string) error { return nil }
func (noopVectorMemory) Search(_ context.Context, _ string, _ int) ([]MemoryItem, error) {
	return nil, nil
}

type fileVectorMemory struct {
	mu    sync.RWMutex
	path  string
	items []MemoryItem
	llm   *llm.LLMClient
}

func NewVectorMemory(path string, llmClient *llm.LLMClient) (VectorMemory, error) {
	if path == "" || llmClient == nil {
		return noopVectorMemory{}, nil
	}
	dir := filepath.Dir(path)
	if dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("create memory dir: %w", err)
		}
	}
	m := &fileVectorMemory{
		path: path,
		llm:  llmClient,
	}
	if err := m.load(); err != nil {
		return nil, err
	}
	return m, nil
}

func (m *fileVectorMemory) load() error {
	f, err := os.OpenFile(m.path, os.O_CREATE|os.O_RDONLY, 0644)
	if err != nil {
		return fmt.Errorf("open memory file: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var item MemoryItem
		if err := json.Unmarshal(line, &item); err == nil {
			m.items = append(m.items, item)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scan memory file: %w", err)
	}
	return nil
}

func (m *fileVectorMemory) appendToFile(item MemoryItem) error {
	f, err := os.OpenFile(m.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open memory file for append: %w", err)
	}
	defer f.Close()

	b, err := json.Marshal(item)
	if err != nil {
		return fmt.Errorf("marshal memory item: %w", err)
	}
	if _, err := f.Write(b); err != nil {
		return fmt.Errorf("write memory item: %w", err)
	}
	if _, err := f.Write([]byte{'\n'}); err != nil {
		return fmt.Errorf("write newline: %w", err)
	}
	return nil
}

func (m *fileVectorMemory) Add(ctx context.Context, taskID, text string) error {
	if text == "" {
		return nil
	}
	vec, err := m.llm.Embed(ctx, text)
	if err != nil {
		return err
	}
	item := MemoryItem{
		ID:        uuid.New().String(),
		TaskID:    taskID,
		Text:      truncate(text, MaxMemoryTextLen),
		Vector:    vec,
		CreatedAt: time.Now().Unix(),
	}

	m.mu.Lock()
	m.items = append(m.items, item)
	m.mu.Unlock()

	return m.appendToFile(item)
}

func (m *fileVectorMemory) Search(ctx context.Context, query string, topK int) ([]MemoryItem, error) {
	if query == "" {
		return nil, nil
	}
	if topK <= 0 {
		topK = 5
	}

	qVec, err := m.llm.Embed(ctx, query)
	if err != nil {
		return nil, err
	}

	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.items) == 0 {
		return nil, nil
	}

	type scored struct {
		item  MemoryItem
		score float32
	}
	results := make([]scored, 0, len(m.items))
	for _, it := range m.items {
		if len(it.Vector) == 0 || len(it.Vector) != len(qVec) {
			continue
		}
		score := llm.CosineSimilarity(qVec, it.Vector)
		if score <= 0 {
			continue
		}
		results = append(results, scored{item: it, score: score})
	}
	if len(results) == 0 {
		return nil, nil
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].score > results[j].score
	})
	if len(results) > topK {
		results = results[:topK]
	}

	out := make([]MemoryItem, 0, len(results))
	for _, r := range results {
		out = append(out, r.item)
	}
	return out, nil
}
