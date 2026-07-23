package cli

import "github.com/matcra587/jira-cli/internal/errtax"

// CodedError attaches an explicit [errtax.Code] to a plain message. A site
// that already knows the correct classification — e.g. a multi-key runner
// that has each failure's typed code in hand — emits the code directly
// instead of leaving [MapError] to guess it from the message text. It
// wraps an optional cause so errors.Is/As still walk the chain.
type CodedError struct {
	code errtax.Code
	msg  string
	err  error
}

// NewCodedError builds a typed error carrying code and message.
func NewCodedError(code errtax.Code, msg string) *CodedError {
	return &CodedError{code: code, msg: msg}
}

// WrapCoded is NewCodedError that also retains cause for errors.Is/As.
func WrapCoded(code errtax.Code, msg string, cause error) *CodedError {
	return &CodedError{code: code, msg: msg, err: cause}
}

func (e *CodedError) Error() string { return e.msg }

// Code is the classification the constructing site attached directly, used by
// MapError instead of guessing one from the message.
func (e *CodedError) Code() errtax.Code { return e.code }

func (e *CodedError) Unwrap() error { return e.err }

var _ errtax.Coded = (*CodedError)(nil) //nolint:errcheck // compile-time interface assertion

// AggregateCode is the errtax code a multi-key partial-failure should
// carry, derived from the worst already-classified per-key failure rather
// than re-guessed from the summary string. Falls back to the type's
// default code, then to validation, so the aggregate is never uncoded.
func AggregateCode(top Error) errtax.Code {
	if top.Code != "" {
		return errtax.Code(top.Code)
	}
	if top.Type != "" {
		return errtax.DefaultCode(errtax.Type(top.Type))
	}
	return errtax.CodeValidationFailed
}
