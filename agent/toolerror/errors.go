package toolerror

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

type ToolExecError struct {
	Err       error
	Retryable bool
}

func (e *ToolExecError) Error() string {
	if e == nil || e.Err == nil {
		return ""
	}
	if e.Retryable {
		return fmt.Sprintf("retryable tool error: %v", e.Err)
	}
	return fmt.Sprintf("fatal tool error: %v", e.Err)
}

func (e *ToolExecError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func ClassifyToolError(err error) *ToolExecError {
	if err == nil {
		return nil
	}
	if te := new(ToolExecError); errors.As(err, &te) {
		return te
	}

	retryable := false
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		retryable = true
	case errors.Is(err, context.Canceled):
		retryable = false
	default:
		msg := strings.ToLower(err.Error())
		retryableHints := []string{
			"timeout",
			"temporar",
			"rate limit",
			"429",
			"503",
			"connection reset",
			"connection refused",
			"eof",
		}
		for _, hint := range retryableHints {
			if strings.Contains(msg, hint) {
				retryable = true
				break
			}
		}
	}

	return &ToolExecError{
		Err:       err,
		Retryable: retryable,
	}
}
