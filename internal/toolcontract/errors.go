package toolcontract

import (
	"errors"
	"fmt"
	"strings"
)

// FailureKind gives the loop a stable control signal without coupling it to a
// concrete tool provider or parsing provider-specific error strings.
type FailureKind string

const (
	FailureRetryable          FailureKind = "retryable"
	FailurePermanent          FailureKind = "permanent"
	FailureApprovalRequired   FailureKind = "approval_required"
	FailureApprovalRejected   FailureKind = "approval_rejected"
	FailureUserActionRequired FailureKind = "user_action_required"
)

// ActionError wraps an execution failure with the control semantics the agent
// loop needs. Cause remains available through errors.Is/errors.As.
type ActionError struct {
	Kind  FailureKind
	Cause error
}

func (e *ActionError) Error() string {
	if e == nil {
		return ""
	}
	if e.Cause == nil {
		return string(e.Kind)
	}
	return e.Cause.Error()
}

func (e *ActionError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func NewActionError(kind FailureKind, cause error) error {
	if cause == nil {
		cause = errors.New(strings.ReplaceAll(string(kind), "_", " "))
	}
	return &ActionError{Kind: kind, Cause: cause}
}

func FailureKindOf(err error) (FailureKind, bool) {
	var actionErr *ActionError
	if !errors.As(err, &actionErr) || actionErr == nil {
		return "", false
	}
	return actionErr.Kind, true
}

func ApprovalRequiredError(tool string) error {
	return NewActionError(FailureApprovalRequired, fmt.Errorf("approval is required before executing high-risk tool %q", strings.TrimSpace(tool)))
}
