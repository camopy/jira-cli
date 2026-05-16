package config

import (
	"errors"
	"fmt"
)

// ErrorType is the broad classification of a credential or auth failure. It
// maps onto the CLI's existing top-level error categories so the output layer
// can pick an exit code without inspecting the specific code.
type ErrorType string

const (
	// ErrorTypeAuth marks a credential or backend failure: the credential is
	// missing, the backend is unreachable, or a provider rejected the request.
	ErrorTypeAuth ErrorType = "auth"
	// ErrorTypeValidation marks a caller-input failure: a conflicting flag
	// combination, an empty credential, or an unsafe profile name.
	ErrorTypeValidation ErrorType = "validation"
)

// ErrorCode is the stable, normalized failure code for a credential or auth
// error. The Go identifier is MixedCaps; the underlying string value is the
// snake_case code that downstream layers serialize on the wire.
type ErrorCode string

const (
	// ErrorCodeCredentialSourceConflict: more than one credential input
	// source was supplied for one operation.
	ErrorCodeCredentialSourceConflict ErrorCode = "credential_source_conflict"
	// ErrorCodeCredentialMissing: no credential is stored or configured for
	// the profile.
	ErrorCodeCredentialMissing ErrorCode = "credential_missing"
	// ErrorCodeCredentialEmpty: a credential was supplied but is empty.
	ErrorCodeCredentialEmpty ErrorCode = "credential_empty"
	// ErrorCodeCredentialBackendUnavailable: the credential backend could not
	// be reached or failed to service the request.
	ErrorCodeCredentialBackendUnavailable ErrorCode = "credential_backend_unavailable"
	// ErrorCodeCredentialMigrationFailed: a backend-switch migration could
	// not complete; no metadata was changed.
	ErrorCodeCredentialMigrationFailed ErrorCode = "credential_migration_failed"
	// ErrorCodeCredentialCleanupFailed: a migration succeeded but the old
	// secret could not be removed from the source backend.
	ErrorCodeCredentialCleanupFailed ErrorCode = "credential_cleanup_failed"
	// ErrorCodeCredentialNamespaceCollision: a profile name cannot be encoded
	// into a credential namespace that round-trips uniquely.
	ErrorCodeCredentialNamespaceCollision ErrorCode = "credential_namespace_collision"
	// ErrorCodeOnePasswordItemAmbiguous: more than one 1Password item matched
	// the configured title.
	ErrorCodeOnePasswordItemAmbiguous ErrorCode = "onepassword_item_ambiguous"
	// ErrorCodeOnePasswordUnavailable: the 1Password SDK or CLI could not be
	// reached or rejected the request.
	ErrorCodeOnePasswordUnavailable ErrorCode = "onepassword_unavailable"
)

// ErrorContext carries optional, fixed-shape context for a credential error.
// Every field is optional; an empty field is simply omitted by the consumer.
// Explicit fields are used rather than a map so the shape is checked at
// compile time and no unknown keys can leak.
type ErrorContext struct {
	// Flag is the CLI flag involved, when the failure is flag-related.
	Flag string
	// ConfigKey is the config key involved, when the failure is config-related.
	ConfigKey string
	// Backend is the credential backend involved (keyring, 1password).
	Backend string
	// CredentialSource is the credential input source involved
	// (stdin, env, prompt, json).
	CredentialSource string
}

// UpstreamProvider carries provider-specific failure metadata for a backend
// that exposes structured values. It is populated only when the provider
// reports a code or status; it is never derived by parsing a human message.
// Provider codes are kept separate from CredentialError.ErrCode so the
// normalized code stays the primary branch target.
type UpstreamProvider struct {
	// Provider names the backend, e.g. "onepassword-sdk".
	Provider string
	// UpstreamCode is a bounded, redacted provider code when one can be parsed
	// safely. Empty when the provider exposes none.
	UpstreamCode string
	// UpstreamStatus is a provider status or process exit code. Zero when the
	// provider exposes none.
	UpstreamStatus int
}

// CredentialError is the concrete typed error for credential and auth
// failures in the config package. It exposes enough metadata for a later
// output layer to render a structured error contract without importing
// internal/cli into internal/config. Build it directly or via the
// classifier; recover it with errors.As and branch on Code, never on the
// message text.
type CredentialError struct {
	// Type is the broad classification (auth or validation).
	Type ErrorType
	// ErrCode is the stable normalized code, the primary branch target.
	ErrCode ErrorCode
	// Message is a display message safe for humans and agents. It never
	// contains a secret.
	Message string
	// HintMsg is a short next-action hint. It never contains a secret and
	// never suggests a mutating command unless the failed command was itself
	// mutating auth state.
	HintMsg string
	// IsRetryable reports whether retrying the same operation could succeed.
	IsRetryable bool
	// Context carries optional, fixed-shape context (flag, config key, etc.).
	Context ErrorContext
	// Upstream carries optional provider metadata when the backend exposes a
	// structured code or status. Nil when none is available.
	Upstream *UpstreamProvider
	// Wrapped is the underlying error, preserved so provider-specific callers
	// can still use errors.As against it.
	Wrapped error
}

// Error implements the error interface. It renders the type, the display
// message, and the hint so the next action survives when the error is shown
// as plain text; the wrapped cause is appended when present.
func (e *CredentialError) Error() string {
	msg := fmt.Sprintf("%s: %s", e.Type, e.Message)
	if e.HintMsg != "" {
		msg += ": " + e.HintMsg
	}
	if e.Wrapped != nil {
		msg += fmt.Sprintf(": %v", e.Wrapped)
	}
	return msg
}

// Unwrap exposes the wrapped upstream error to errors.Is / errors.As.
func (e *CredentialError) Unwrap() error { return e.Wrapped }

// Code returns the stable normalized failure code.
func (e *CredentialError) Code() ErrorCode { return e.ErrCode }

// Hint returns the short next-action hint.
func (e *CredentialError) Hint() string { return e.HintMsg }

// Retryable reports whether retrying the operation could succeed.
func (e *CredentialError) Retryable() bool { return e.IsRetryable }

// ClassifyCredentialError maps an error returned by a credential backend onto
// a typed CredentialError. It branches on typed errors — never on message
// substrings — so a backend changing its wording cannot reclassify a failure.
// A nil error returns nil.
//
// The display message and hint are fixed strings: the upstream error is kept
// only as the wrapped cause, never copied into the message, so an upstream
// error that embeds a secret cannot leak through the classified error.
func ClassifyCredentialError(err error, backend string) *CredentialError {
	if err == nil {
		return nil
	}
	var alreadyTyped *CredentialError
	if errors.As(err, &alreadyTyped) {
		return alreadyTyped
	}
	if errors.Is(err, ErrCredentialNotFound) {
		return &CredentialError{
			Type:        ErrorTypeAuth,
			ErrCode:     ErrorCodeCredentialMissing,
			Message:     "no credential is stored for this profile",
			HintMsg:     "store a credential for this profile, then retry",
			IsRetryable: false,
			Context:     ErrorContext{Backend: backend},
			Wrapped:     err,
		}
	}
	if errors.Is(err, ErrCredentialEmpty) {
		return &CredentialError{
			Type:        ErrorTypeValidation,
			ErrCode:     ErrorCodeCredentialEmpty,
			Message:     "the supplied credential is empty",
			HintMsg:     "supply a non-empty credential",
			IsRetryable: false,
			Context:     ErrorContext{Backend: backend},
			Wrapped:     err,
		}
	}
	// Any other backend error is a backend-unavailable failure. The upstream
	// error is wrapped, not copied into the message, so it cannot leak a
	// secret embedded in backend wording.
	return &CredentialError{
		Type:        ErrorTypeAuth,
		ErrCode:     ErrorCodeCredentialBackendUnavailable,
		Message:     fmt.Sprintf("the %s credential backend is unavailable", backendLabel(backend)),
		HintMsg:     "check that the credential backend is running and reachable, then retry",
		IsRetryable: true,
		Context:     ErrorContext{Backend: backend},
		Wrapped:     err,
	}
}

func backendLabel(backend string) string {
	if backend == "" {
		return "credential"
	}
	return backend
}
