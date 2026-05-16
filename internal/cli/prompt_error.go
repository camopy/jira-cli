package cli

import "fmt"

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

// promptCode is the stable snake_case envelope code for a prompt kind.
func (e *PromptError) promptCode() string {
	switch e.Kind {
	case PromptCanceled:
		return "prompt_canceled"
	case PromptUnavailable:
		return "prompt_unavailable"
	default:
		return "prompt_aborted"
	}
}

// promptHint is the next-action hint for a prompt failure.
func (e *PromptError) promptHint() string {
	switch e.Kind {
	case PromptCanceled:
		return "The prompt was interrupted; rerun the command when ready."
	case PromptUnavailable:
		return "Provide the value via a flag or --json-input so no prompt is needed."
	default:
		return "Rerun and complete the prompt, or supply the value via a flag."
	}
}
