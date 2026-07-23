package cli

import (
	"github.com/matcra587/jira-cli/internal/errtax"
)

// NotFoundError is the typed not_found (exit 2) wrapper for a resource the
// command proved absent — e.g. a live /user/search that returned zero
// matches. It wraps the cause so errors.Is still recognizes sentinels like
// jira.ErrUserNotFound, and carries the stable code so MapError never
// falls back to the substring classifier.
type NotFoundError struct {
	// Message is the user-facing diagnosis; when empty the wrapped
	// error's text is used verbatim.
	Message string
	Err     error
}

// NewNotFoundError wraps err as a typed not_found failure. An empty
// message keeps the wrapped error's text.
func NewNotFoundError(message string, err error) *NotFoundError {
	return &NotFoundError{Message: message, Err: err}
}

func (e *NotFoundError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	return "not found"
}

func (e *NotFoundError) Unwrap() error { return e.Err }

// Code is the stable not_found envelope code (exit 2).
func (e *NotFoundError) Code() errtax.Code { return errtax.CodeNotFound }

var _ errtax.Coded = (*NotFoundError)(nil) //nolint:errcheck // compile-time interface assertion
