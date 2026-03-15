package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	capgraph "github.com/OctoSucker/octosucker-utils/graph"
	"github.com/OctoSucker/octosucker/agent/llm"
	"github.com/OctoSucker/octosucker/agent/memory"
	"github.com/OctoSucker/octosucker/agent/planner"
	"github.com/OctoSucker/octosucker/agent/toolerror"
	"github.com/openai/openai-go"
)

func (r *AgentRuntime) runReActLoop(ctx context.Context, task *Task) {
	taskMemory := NewTaskMemory(task.Input)

	var graph *capgraph.Graph
	g, err := r.planner.Plan(ctx, task.Input)
	if err == nil && g != nil {
		graph = g
	} else {
		graph = capgraph.NewGraphFromNodes(r.capabilityRegistry.CopyNodes())
		var initialNames []string
		if r.selector != nil {
			nodes := r.selector.Select(task.Input, planner.DefaultSelectK)
			if len(nodes) > 0 {
				initialNames = planner.NodeNames(nodes)
			}
		}
		if len(initialNames) == 0 {
			initialNames = r.planner.EntryCapabilities()
		}
		graph.SetCurrent(initialNames)
	}

	memorySummary := ""
	if r.memory != nil {
		memorySummary = memory.BuildSummaryForQuery(ctx, r.memory, task.Input, 5, log.Printf)
	}

	for step := 0; step < r.maxReActIterations; step++ {
		select {
		case <-ctx.Done():
			log.Printf("[agent] ReAct cancelled: task=%s err=%v", task.ID, ctx.Err())
			return
		default:
		}

		nodes := graph.CurrentNodes()
		if len(nodes) == 0 && r.generator != nil && !graph.HistoryRepeatedTooMuch() && len(graph.Nodes) < graph.MaxNodes {
			existingNames := make([]string, 0, len(graph.Nodes))
			for name := range graph.Nodes {
				existingNames = append(existingNames, name)
			}
			dynamicNode, err := r.generator.Generate(ctx, task.Input, existingNames, r.toolRegistry.GetToolNames())
			if err != nil {
				log.Printf("[agent] dynamic generate failed: %v", err)
			} else if graph.AddDynamicNode(dynamicNode) {
				log.Printf("[agent] added dynamic capability: %s -> %s", dynamicNode.Name, dynamicNode.Tool)
				continue
			}
		}

		toolDefs := capgraph.ToolDefsFromGraph(graph)
		fullMessages := []openai.ChatCompletionMessageParamUnion{openai.SystemMessage(r.systemPrompt)}
		if memorySummary != "" {
			fullMessages = append(fullMessages, openai.SystemMessage("下面是与你当前任务可能相关的历史记忆（向量检索得到）：\n"+memorySummary))
		}
		fullMessages = append(fullMessages, taskMemory.Messages()...)

		resp, err := r.llmClient.ChatCompletionWithTools(ctx, fullMessages, toolDefs)
		if err != nil {
			log.Printf("[agent] LLM call failed: %v", err)
			return
		}

		if len(resp.ToolCalls) > 0 {
			toolCallsData := make([]map[string]interface{}, 0, len(resp.ToolCalls))
			for _, tc := range resp.ToolCalls {
				argsJSON, _ := json.Marshal(tc.Arguments)
				toolCallsData = append(toolCallsData, map[string]interface{}{
					"id":   tc.ID,
					"type": "function",
					"function": map[string]interface{}{
						"name":      tc.Name,
						"arguments": string(argsJSON),
					},
				})
			}
			taskMemory.AddAssistantToolCalls(toolCallsData)
		}

		actions := llm.ActionsFromResult(resp)
		for _, action := range actions {
			switch action.Type {
			case llm.ActionTypeComplete:
				if action.Content != "" {
					taskMemory.AddAssistantMessage(action.Content)
					log.Printf("[agent] Task %s final reply: %s", task.ID, action.Content)
					if r.memory != nil {
						_ = r.memory.Add(ctx, task.ID, memory.FormatTaskAnswer(task.Input, action.Content))
					}
				}
				return
			case llm.ActionTypeTool:
			default:
				continue
			}

			call := action.Tool
			if call == nil {
				continue
			}
			node := graph.GetNode(call.Name)
			if node == nil {
				node, _ = r.capabilityRegistry.Get(call.Name)
			}
			if node == nil {
				node, _ = r.capabilityRegistry.GetByTool(call.Name)
			}
			if node == nil {
				capNotFoundErr, _ := json.Marshal(map[string]interface{}{"success": false, "error": fmt.Sprintf("capability not found: %s", call.Name)})
				taskMemory.AddToolResult(call.ID, string(capNotFoundErr))
				continue
			}
			args := call.Arguments
			if args == nil {
				args = make(map[string]interface{})
			}

			maxRetries := r.maxToolRetries
			if maxRetries < 0 {
				maxRetries = 0
			}
			var result interface{}
			var execErr error
			for attempt := 0; attempt <= maxRetries; attempt++ {
				attemptCtx := ctx
				var cancel context.CancelFunc
				if r.toolTimeout > 0 {
					attemptCtx, cancel = context.WithTimeout(ctx, r.toolTimeout)
				}
				if node.Tool == "" {
					result = map[string]interface{}{"done": true}
				} else {
					argsJSON := ""
					if len(args) > 0 {
						b, _ := json.Marshal(args)
						argsJSON = string(b)
					}
					result, execErr = r.ExecuteTool(attemptCtx, node.Tool, argsJSON)
				}
				if cancel != nil {
					cancel()
				}
				if execErr == nil {
					break
				}
				classified := toolerror.ClassifyToolError(execErr)
				if classified == nil || !classified.Retryable || attempt == maxRetries {
					if classified != nil {
						execErr = classified
					}
					break
				}
				backoff := time.Duration(attempt+1) * 150 * time.Millisecond
				timer := time.NewTimer(backoff)
				select {
				case <-ctx.Done():
					timer.Stop()
					execErr = toolerror.ClassifyToolError(ctx.Err())
					attempt = maxRetries
				case <-timer.C:
				}
			}
			if execErr != nil {
				result = nil
			}

			var resultStr string
			if execErr != nil {
				b, _ := json.Marshal(map[string]interface{}{"success": false, "error": execErr.Error()})
				resultStr = string(b)
			} else {
				b, _ := json.Marshal(result)
				resultStr = string(b)
			}
			taskMemory.AddToolResult(call.ID, resultStr)
			if r.memory != nil {
				argsPreview, _ := json.Marshal(call.Arguments)
				_ = r.memory.Add(ctx, task.ID, memory.FormatToolStep(task.Input, call.Name, string(argsPreview), fmt.Sprint(result), execErr))
			}
			graph.Advance(node.Name)
		}
	}
}
