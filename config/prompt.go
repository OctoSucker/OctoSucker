package config

import (
	"os"
	"strings"
)

const (
	defaultSystemPromptPath = "workspace/system_prompt.md"
	defaultStartupTasksPath = "workspace/startup_tasks.md"
)

type SystemPromptConfig struct {
	SystemPrompt string   `json:"system_prompt"`
	StartupTasks []string `json:"startup_tasks,omitempty"`
}

func LoadSystemPrompt() (SystemPromptConfig, error) {
	var out SystemPromptConfig

	systemPrompt, err := os.ReadFile(defaultSystemPromptPath)
	if err != nil {
		return out, err
	}
	out.SystemPrompt = strings.TrimSpace(string(systemPrompt))

	tasksRaw, err := os.ReadFile(defaultStartupTasksPath)
	if err != nil {
		return out, err
	}
	blocks := strings.Split(string(tasksRaw), "\n---\n")
	for _, b := range blocks {
		s := strings.TrimSpace(b)
		if s != "" {
			out.StartupTasks = append(out.StartupTasks, s)
		}
	}

	return out, nil
}
