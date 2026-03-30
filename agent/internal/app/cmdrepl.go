package app

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

const cmdInputPrompt = "🐙 > "

// RunCmdREPL reads lines from stdin and dispatches them through RunInputLocal until ctx is done or EOF.
func RunCmdREPL(ctx context.Context, a *App, logPath string) error {
	fmt.Fprintf(os.Stdout, "cmd: 输入消息与 agent 对话（详细日志: %s，可用 tail -f 查看）\n", logPath)
	rd := bufio.NewReader(os.Stdin)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		fmt.Fprint(os.Stdout, cmdInputPrompt)
		line, err := rd.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		msgs, err := a.RunInputLocal(ctx, line)
		if err != nil {
			fmt.Fprintf(os.Stdout, "error: %v\n", err)
			continue
		}
		for _, m := range msgs {
			fmt.Fprintln(os.Stdout, m)
		}
	}
}
