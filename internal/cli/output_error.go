package cli

import (
	"fmt"

	"github.com/matcra587/jira-cli/internal/errtax"
)

// OutputError reports that the CLI could not write command output to its
// destination. The operation may already have completed, so callers must not
// infer that retrying the command is safe.
type OutputError struct {
	Err error
}

// NewOutputError wraps a destination write failure for stable error mapping.
func NewOutputError(err error) *OutputError {
	return &OutputError{Err: err}
}

func (e *OutputError) Error() string {
	if e == nil || e.Err == nil {
		return "write output"
	}
	return fmt.Sprintf("write output: %v", e.Err)
}

func (e *OutputError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// Code identifies a local output destination failure.
func (e *OutputError) Code() errtax.Code {
	return errtax.CodeOutputWriteFailed
}

var _ errtax.Coded = (*OutputError)(nil)
