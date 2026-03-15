package llm

type ActionType string

const (
	ActionTypeTool     ActionType = "tool"
	ActionTypeComplete ActionType = "complete"
)

type Action struct {
	Type    ActionType
	Tool    *ToolCall
	Content string
}

func ActionsFromResult(resp *ChatCompletionResult) []Action {
	if resp == nil {
		return nil
	}
	if !resp.HasToolCalls() {
		return []Action{{
			Type:    ActionTypeComplete,
			Content: resp.Content,
		}}
	}

	actions := make([]Action, 0, len(resp.ToolCalls))
	for i := range resp.ToolCalls {
		call := resp.ToolCalls[i]
		actions = append(actions, Action{
			Type: ActionTypeTool,
			Tool: &call,
		})
	}
	return actions
}
