package memory

import "fmt"

const (
	PrefixInput  = "input:"
	PrefixTool   = "tool:"
	PrefixArgs   = "args:"
	PrefixResult = "result:"
	PrefixErr    = "err:"
	PrefixAnswer = "answer:"
)

func FormatToolStep(taskInput, toolName, args, result string, err error) string {
	return fmt.Sprintf("%s %s\n%s %s\n%s %s\n%s %s\n%s %v",
		PrefixInput, taskInput,
		PrefixTool, toolName,
		PrefixArgs, args,
		PrefixResult, result,
		PrefixErr, err)
}

func FormatTaskAnswer(taskInput, answer string) string {
	return fmt.Sprintf("%s %s\n%s %s", PrefixInput, taskInput, PrefixAnswer, answer)
}
