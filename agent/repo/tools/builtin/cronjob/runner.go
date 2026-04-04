package cronjob

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/OctoSucker/agent/pkg/ports"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/robfig/cron/v3"
)

const (
	ToolCreateJob = "create_cronjob"
	ToolDeleteJob = "delete_cronjob"
	ToolListJobs  = "list_cronjobs"
)

type Task struct {
	ID      string `json:"id"`
	Spec    string `json:"spec"`
	Content string `json:"content"`
}

type Runner struct {
	workspaceDir string
	storePath    string
	cron         *cron.Cron

	mu    sync.Mutex
	tasks map[string]Task
	ids   map[string]cron.EntryID
}

func NewRunner(workspaceDir string) (*Runner, error) {
	wd := strings.TrimSpace(workspaceDir)
	if wd == "" {
		return nil, fmt.Errorf("cronjob builtin: workspace dir is required")
	}
	abs, err := filepath.Abs(wd)
	if err != nil {
		return nil, fmt.Errorf("cronjob builtin: resolve workspace dir: %w", err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, fmt.Errorf("cronjob builtin: workspace dir %q: %w", abs, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("cronjob builtin: workspace dir %q is not a directory", abs)
	}

	parser := cron.NewParser(
		cron.SecondOptional | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor,
	)
	c := cron.New(cron.WithParser(parser))
	r := &Runner{
		workspaceDir: abs,
		storePath:    filepath.Join(abs, "cronjob.md"),
		cron:         c,
		tasks:        map[string]Task{},
		ids:          map[string]cron.EntryID{},
	}
	if err := r.loadAndSchedule(); err != nil {
		return nil, err
	}
	r.cron.Start()
	return r, nil
}

// Name is the ToolRegistry.Backends map key for this provider (not a user-facing tool id).
func (r *Runner) Name() string { return "cronjob" }

func (r *Runner) HasTool(name string) bool {
	switch strings.TrimSpace(name) {
	case ToolCreateJob, ToolDeleteJob, ToolListJobs:
		return true
	default:
		return false
	}
}

func (r *Runner) ToolList(ctx context.Context) ([]*mcp.Tool, error) {
	return []*mcp.Tool{
		{
			Name:        ToolCreateJob,
			Description: "Create and persist a cron task in workspace/cronjob.md, then schedule it immediately.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id": map[string]any{
						"type":        "string",
						"description": "Unique task id",
					},
					"spec": map[string]any{
						"type":        "string",
						"description": "Cron expression, supports optional seconds field",
					},
					"content": map[string]any{
						"type":        "string",
						"description": "Task description or instruction text",
					},
				},
				"required":             []any{"id", "spec", "content"},
				"additionalProperties": false,
			},
		},
		{
			Name:        ToolDeleteJob,
			Description: "Delete an existing cron task from memory and workspace/cronjob.md.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id": map[string]any{
						"type":        "string",
						"description": "Task id to delete",
					},
				},
				"required":             []any{"id"},
				"additionalProperties": false,
			},
		},
		{
			Name:        ToolListJobs,
			Description: "List all currently scheduled cron tasks in memory.",
			InputSchema: map[string]any{
				"type":                 "object",
				"properties":           map[string]any{},
				"additionalProperties": false,
			},
		},
	}, nil
}

func (r *Runner) Tool(tool string) (*mcp.Tool, error) {
	tools, err := r.ToolList(context.Background())
	if err != nil {
		return nil, err
	}
	for _, t := range tools {
		if t != nil && t.Name == tool {
			return t, nil
		}
	}
	return nil, fmt.Errorf("cronjob builtin: unknown tool %q", tool)
}

func (r *Runner) Invoke(ctx context.Context, localTool string, arguments map[string]any) (ports.ToolResult, error) {
	switch localTool {
	case ToolCreateJob:
		task, err := parseCreateArgs(arguments)
		if err != nil {
			return ports.ToolResult{Err: err}, err
		}
		if err := r.createTask(task); err != nil {
			return ports.ToolResult{Err: err}, err
		}
		return ports.ToolResult{
			Output: map[string]any{
				"created": task.ID,
				"spec":    task.Spec,
			},
		}, nil
	case ToolDeleteJob:
		id, err := parseDeleteArgs(arguments)
		if err != nil {
			return ports.ToolResult{Err: err}, err
		}
		if err := r.deleteTask(id); err != nil {
			return ports.ToolResult{Err: err}, err
		}
		return ports.ToolResult{
			Output: map[string]any{
				"deleted": id,
			},
		}, nil
	case ToolListJobs:
		jobs := r.listTasks()
		return ports.ToolResult{
			Output: map[string]any{
				"jobs": jobs,
			},
		}, nil
	default:
		return ports.ToolResult{Err: fmt.Errorf("cronjob builtin: unknown tool %q", localTool)}, fmt.Errorf("cronjob builtin: unknown tool %q", localTool)
	}
}

func (r *Runner) listTasks() []map[string]any {
	r.mu.Lock()
	defer r.mu.Unlock()
	ids := make([]string, 0, len(r.tasks))
	for id := range r.tasks {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]map[string]any, 0, len(ids))
	for _, id := range ids {
		task := r.tasks[id]
		entryID, ok := r.ids[id]
		if !ok {
			continue
		}
		entry := r.cron.Entry(entryID)
		out = append(out, map[string]any{
			"id":      task.ID,
			"spec":    task.Spec,
			"content": task.Content,
			"next":    entry.Next.Format(time.RFC3339),
			"prev":    entry.Prev.Format(time.RFC3339),
		})
	}
	return out
}

func parseCreateArgs(args map[string]any) (Task, error) {
	if args == nil {
		return Task{}, fmt.Errorf("cronjob builtin: create_cronjob: arguments required")
	}
	getString := func(k string) (string, error) {
		raw, ok := args[k]
		if !ok {
			return "", fmt.Errorf("cronjob builtin: create_cronjob: %s is required", k)
		}
		v, ok := raw.(string)
		if !ok {
			return "", fmt.Errorf("cronjob builtin: create_cronjob: %s must be string", k)
		}
		v = strings.TrimSpace(v)
		if v == "" {
			return "", fmt.Errorf("cronjob builtin: create_cronjob: %s must be non-empty", k)
		}
		return v, nil
	}
	id, err := getString("id")
	if err != nil {
		return Task{}, err
	}
	spec, err := getString("spec")
	if err != nil {
		return Task{}, err
	}
	content, err := getString("content")
	if err != nil {
		return Task{}, err
	}
	return Task{ID: id, Spec: spec, Content: content}, nil
}

func parseDeleteArgs(args map[string]any) (string, error) {
	if args == nil {
		return "", fmt.Errorf("cronjob builtin: delete_cronjob: arguments required")
	}
	raw, ok := args["id"]
	if !ok {
		return "", fmt.Errorf("cronjob builtin: delete_cronjob: id is required")
	}
	id, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("cronjob builtin: delete_cronjob: id must be string")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return "", fmt.Errorf("cronjob builtin: delete_cronjob: id must be non-empty")
	}
	return id, nil
}

func (r *Runner) createTask(task Task) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.tasks[task.ID]; exists {
		return fmt.Errorf("cronjob builtin: create_cronjob: id %q already exists", task.ID)
	}
	entryID, err := r.addCronEntry(task)
	if err != nil {
		return err
	}
	r.tasks[task.ID] = task
	r.ids[task.ID] = entryID
	if err := r.persistLocked(); err != nil {
		r.cron.Remove(entryID)
		delete(r.tasks, task.ID)
		delete(r.ids, task.ID)
		return err
	}
	return nil
}

func (r *Runner) deleteTask(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	entryID, ok := r.ids[id]
	if !ok {
		return fmt.Errorf("cronjob builtin: delete_cronjob: id %q not found", id)
	}
	r.cron.Remove(entryID)
	delete(r.ids, id)
	delete(r.tasks, id)
	if err := r.persistLocked(); err != nil {
		return err
	}
	return nil
}

func (r *Runner) addCronEntry(task Task) (cron.EntryID, error) {
	entryID, err := r.cron.AddFunc(task.Spec, func() {
		log.Printf("cronjob builtin: trigger id=%s spec=%s content=%q", task.ID, task.Spec, task.Content)
	})
	if err != nil {
		return 0, fmt.Errorf("cronjob builtin: invalid cron spec for id %q: %w", task.ID, err)
	}
	return entryID, nil
}

func (r *Runner) loadAndSchedule() error {
	tasks, err := r.loadFromDisk()
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, task := range tasks {
		if _, exists := r.tasks[task.ID]; exists {
			return fmt.Errorf("cronjob builtin: duplicate id %q in cronjob.md", task.ID)
		}
		entryID, err := r.addCronEntry(task)
		if err != nil {
			return err
		}
		r.tasks[task.ID] = task
		r.ids[task.ID] = entryID
	}
	return nil
}

func (r *Runner) loadFromDisk() ([]Task, error) {
	if _, err := os.Stat(r.storePath); err != nil {
		if os.IsNotExist(err) {
			if err := os.WriteFile(r.storePath, []byte(defaultMarkdown()), 0644); err != nil {
				return nil, fmt.Errorf("cronjob builtin: create %s: %w", r.storePath, err)
			}
			return nil, nil
		}
		return nil, fmt.Errorf("cronjob builtin: stat %s: %w", r.storePath, err)
	}
	raw, err := os.ReadFile(r.storePath)
	if err != nil {
		return nil, fmt.Errorf("cronjob builtin: read %s: %w", r.storePath, err)
	}
	list, err := decodeTasksMarkdown(raw)
	if err != nil {
		return nil, fmt.Errorf("cronjob builtin: parse %s: %w", r.storePath, err)
	}
	return list, nil
}

func (r *Runner) persistLocked() error {
	out, err := encodeTasksMarkdown(r.tasks)
	if err != nil {
		return err
	}
	if err := os.WriteFile(r.storePath, out, 0644); err != nil {
		return fmt.Errorf("cronjob builtin: write %s: %w", r.storePath, err)
	}
	return nil
}

func defaultMarkdown() string {
	return "# Cron Jobs\n\n```json\n[]\n```\n"
}

func decodeTasksMarkdown(raw []byte) ([]Task, error) {
	content := string(raw)
	startFence := "```json"
	endFence := "```"
	start := strings.Index(content, startFence)
	if start < 0 {
		return nil, fmt.Errorf("missing ```json fenced block")
	}
	start += len(startFence)
	end := strings.Index(content[start:], endFence)
	if end < 0 {
		return nil, fmt.Errorf("missing closing fenced block")
	}
	body := strings.TrimSpace(content[start : start+end])
	if body == "" {
		return nil, nil
	}
	var tasks []Task
	if err := json.Unmarshal([]byte(body), &tasks); err != nil {
		return nil, err
	}
	return tasks, nil
}

func encodeTasksMarkdown(tasks map[string]Task) ([]byte, error) {
	list := make([]Task, 0, len(tasks))
	for _, t := range tasks {
		list = append(list, t)
	}
	b, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("cronjob builtin: marshal tasks: %w", err)
	}
	doc := "# Cron Jobs\n\n```json\n" + string(b) + "\n```\n"
	return []byte(doc), nil
}
