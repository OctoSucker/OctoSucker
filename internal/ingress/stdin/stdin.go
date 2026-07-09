// Package stdin is the local terminal ingress: read lines from stdin, invoke a callback, print replies.
// Wired by internal/gateway when local CMD input is enabled.
package stdin

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

// OnMessage handles one non-empty user line; returns lines to print to stdout.
type OnMessage func(ctx context.Context, text string) ([]string, error)

// Run reads stdin line-by-line and passes each line to onMessage until ctx is canceled or EOF.
func Run(ctx context.Context, onMessage OnMessage, logPath string) error {
	fmt.Fprintf(os.Stdout, "cmd: 输入消息进行对话（详细日志: %s，可用 tail -f 查看）\n", logPath)
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
		msgs, err := onMessage(ctx, line)
		if err != nil {
			fmt.Fprintf(os.Stdout, "error: %v\n", err)
			continue
		}
		for _, m := range msgs {
			fmt.Fprintln(os.Stdout, m)
		}
	}
}
