package cli

import (
	"fmt"

	"github.com/matcra587/jira-cli/internal/errtax"
)

// PromptKind names why an interactive prompt did not yield a value.
type PromptKind int

const (
	// PromptAborted means the user dismissed the prompt (Esc / Ctrl-C in
	// the form) before submitting.
	PromptAborted PromptKind = iota
	// PromptCanceled means the prompt was canceled by the command
	// context — a SIGINT or an elapsed --timeout.
	PromptCanceled
	// PromptUnavailable means a prompt was required but could not be
	// shown — no TTY, or --no-input was set.
	PromptUnavailable
)

// PromptError is the typed error every interactive-prompt failure is
// wrapped in before it leaves a command handler. It exists so MapError
// classifies prompt outcomes via errors.As instead of letting the
// substring classifier misread "auth login aborted" as an auth failure.
type PromptError struct {
	Kind   PromptKind
	Prompt string // the prompt that failed, e.g. "auth login"
	Err    error  // the underlying cause (huh.ErrUserAborted, context.Canceled, …)
}

// NewPromptError builds a PromptError for the given prompt and cause.
func NewPromptError(kind PromptKind, prompt string, err error) *PromptError {
	return &PromptError{Kind: kind, Prompt: prompt, Err: err}
}

func (e *PromptError) Error() string {
	switch e.Kind {
	case PromptCanceled:
		return fmt.Sprintf("%s prompt canceled", e.Prompt)
	case PromptUnavailable:
		return fmt.Sprintf("%s requires an interactive prompt, which is unavailable", e.Prompt)
	default:
		return fmt.Sprintf("%s aborted by user", e.Prompt)
	}
}

func (e *PromptError) Unwrap() error { return e.Err }

// Code is the stable snake_case envelope code for a prompt kind.
func (e *PromptError) Code() errtax.Code {
	switch e.Kind {
	case PromptCanceled:
		return errtax.CodePromptCanceled
	case PromptUnavailable:
		return errtax.CodePromptUnavailable
	default:
		return errtax.CodePromptAborted
	}
}

var _ errtax.Coded = (*PromptError)(nil)
