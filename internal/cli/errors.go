package cli

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/matcra587/jira-cli/internal/config"
	"github.com/matcra587/jira-cli/pkg/jira"
)

type ErrorType string

const (
	ErrorTypeAuth       ErrorType = "auth"
	ErrorTypeNotFound   ErrorType = "not_found"
	ErrorTypeValidation ErrorType = "validation"
	ErrorTypeRateLimit  ErrorType = "rate_limit"
	ErrorTypeServer     ErrorType = "server"
)

// NewError builds a structured Error with a normalized code derived from
// the type. Callers with a more specific code should set Error.Code
// directly afterward.
func NewError(kind ErrorType, msg string) Error {
	return Error{Type: string(kind), Code: defaultCodeForType(kind), Message: msg, Retryable: kind == ErrorTypeRateLimit}
}

// defaultCodeForType is the fallback stable code when no more specific
// code is available. Agents branch on Code, so every error carries one.
func defaultCodeForType(kind ErrorType) string {
	switch kind {
	case ErrorTypeAuth:
		return "auth_failed"
	case ErrorTypeNotFound:
		return "not_found"
	case ErrorTypeValidation:
		return "validation_failed"
	case ErrorTypeRateLimit:
		return "rate_limited"
	case ErrorTypeServer:
		return "server_error"
	default:
		return "error"
	}
}

func ExitCode(err Error) int {
	switch ErrorType(err.Type) {
	case ErrorTypeAuth:
		return 1
	case ErrorTypeNotFound:
		return 2
	case ErrorTypeValidation:
		return 3
	case ErrorTypeRateLimit:
		return 4
	case ErrorTypeServer:
		return 5
	default:
		return 5
	}
}

// codedError is the adapter the mapper consumes for any typed error that
// carries a stable code, a next-action hint, and a retryable flag. Phase
// 02's *config.CredentialError satisfies this.
type codedError interface {
	Code() config.ErrorCode
	Hint() string
	Retryable() bool
}

// ValidationCandidatesError is the adapter MapError consumes for any
// command-local typed error that is a user-input validation failure and
// may carry structured disambiguation candidates. The board-resolution
// wrapper in cmd/jira satisfies this; routing it through MapError keeps
// every error envelope built one way. Typed errors must never reach the
// substring classifier, where a message such as "not found" would be
// misclassified into the wrong type and exit code.
type ValidationCandidatesError interface {
	error
	BoardCandidates() []map[string]any
}

// MapError is the central error mapper. It adapts typed source errors
// into a structured cli.Error using errors.As — never substring
// matching where a typed error exists. A bare error falls back to a
// best-effort substring classifier for legacy untyped error strings.
func MapError(err error) Error {
	if err == nil {
		return Error{}
	}
	if out, ok := mapPromptError(err); ok {
		return out
	}
	if out, ok := mapContextError(err); ok {
		return out
	}
	if out, ok := mapCredentialError(err); ok {
		return out
	}
	if out, ok := mapJiraAPIError(err); ok {
		return out
	}
	if out, ok := mapValidationCandidatesError(err); ok {
		return out
	}
	if out, ok := mapAmbiguousUserError(err); ok {
		return out
	}
	return classifyUntyped(err)
}

// mapPromptError adapts a *PromptError. Every interactive-prompt
// outcome — user abort, SIGINT/timeout cancellation, or an unavailable
// prompt — is a validation-class failure (exit 3): the command could
// not gather the input it needed. Recognizing it here via errors.As
// keeps it off the substring classifier, where "auth login aborted"
// would be misbucketed as an auth failure. It must be checked before
// mapContextError so a canceled prompt keeps its prompt identity rather
// than collapsing into the generic timeout/canceled mapping.
func mapPromptError(err error) (Error, bool) {
	var pe *PromptError
	if !errors.As(err, &pe) {
		return Error{}, false
	}
	out := NewError(ErrorTypeValidation, pe.Error())
	out.Code = pe.promptCode()
	out.Hint = pe.promptHint()
	return out, true
}

// mapContextError adapts a context cancellation or deadline error into a
// typed envelope entry. Cancellation reaches MapError when --timeout
// elapses or a SIGINT cancels the root context mid-request; classifying
// it here with errors.Is keeps it off the substring classifier, where
// the wrapped url.Error text would land in the wrong bucket. Both cases
// are retryable: the request never completed.
func mapContextError(err error) (Error, bool) {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		out := NewError(ErrorTypeServer, err.Error())
		out.Code = "timeout"
		out.Hint = "The invocation exceeded its --timeout deadline; raise --timeout or retry."
		out.Retryable = true
		return out, true
	case errors.Is(err, context.Canceled):
		out := NewError(ErrorTypeServer, err.Error())
		out.Code = "canceled"
		out.Hint = "The invocation was canceled before it completed; retry when ready."
		out.Retryable = true
		return out, true
	default:
		return Error{}, false
	}
}

// mapCredentialError adapts a Phase 02 *config.CredentialError. The
// typed error supplies code/hint/retryable directly; provider metadata
// is preserved verbatim when present.
func mapCredentialError(err error) (Error, bool) {
	var ce *config.CredentialError
	if !errors.As(err, &ce) {
		return Error{}, false
	}
	kind := ErrorTypeAuth
	if ce.Type == config.ErrorTypeValidation {
		kind = ErrorTypeValidation
	}
	var adapter codedError = ce
	out := Error{
		Type:      string(kind),
		Code:      string(adapter.Code()),
		Message:   ce.Message,
		Hint:      adapter.Hint(),
		Retryable: adapter.Retryable(),
		Flag:      ce.Context.Flag,
	}
	if ce.Upstream != nil {
		out.Provider = ce.Upstream.Provider
		out.UpstreamCode = ce.Upstream.UpstreamCode
		out.UpstreamStatus = ce.Upstream.UpstreamStatus
	}
	return out, true
}

// mapJiraAPIError adapts a *jira.APIError. Jira exposes no stable
// machine error code, so upstream_code stays empty; the normalized code
// is derived from the HTTP status. Schema-backed ErrorCollection fields
// (errorMessages, errors map, status) and the Retry-After header are
// preserved as optional upstream metadata.
func mapJiraAPIError(err error) (Error, bool) {
	var apiErr *jira.APIError
	if !errors.As(err, &apiErr) {
		return Error{}, false
	}
	kind := ErrorType(apiErr.Type)
	out := Error{
		Type:                string(kind),
		Code:                jiraCodeForStatus(apiErr.StatusCode, kind),
		Message:             apiErr.Message,
		Hint:                jiraHintForStatus(apiErr.StatusCode, kind),
		Retryable:           kind == ErrorTypeRateLimit || kind == ErrorTypeServer,
		HTTPStatus:          apiErr.StatusCode,
		RetryAfterSeconds:   apiErr.RetryAfterSeconds,
		RateLimitRemaining:  apiErr.RateLimitRemaining,
		Provider:            "jira",
		UpstreamStatus:      apiErr.UpstreamStatus,
		UpstreamMessages:    apiErr.ErrorMessages,
		UpstreamFieldErrors: apiErr.FieldErrors,
		// UpstreamCode intentionally left empty: Jira exposes no stable
		// machine-readable error code.
	}
	return out, true
}

// mapValidationCandidatesError adapts a command-local typed validation
// error (board resolution). The error is always exit-3 validation; its
// disambiguation candidates, when present, are preserved verbatim. This
// keeps the board failure off the substring classifier, where the
// missing-default-board message ("... not found ...") would otherwise be
// misread as a not_found/exit-2 failure.
func mapValidationCandidatesError(err error) (Error, bool) {
	var vce ValidationCandidatesError
	if !errors.As(err, &vce) {
		return Error{}, false
	}
	out := NewError(ErrorTypeValidation, vce.Error())
	if cands := vce.BoardCandidates(); len(cands) > 0 {
		out.Candidates = cands
	}
	return out, true
}

// mapAmbiguousUserError adapts a *jira.AmbiguousUserError into the
// structured validation shape (exit 3). The /user/search candidates are
// flattened into the envelope's candidate rows so an agent can pick a
// winner without re-querying.
func mapAmbiguousUserError(err error) (Error, bool) {
	var ambig *jira.AmbiguousUserError
	if !errors.As(err, &ambig) {
		return Error{}, false
	}
	out := NewError(ErrorTypeValidation, ambig.Error())
	cands := make([]map[string]any, 0, len(ambig.Candidates))
	for _, c := range ambig.Candidates {
		if c == nil {
			continue
		}
		row := map[string]any{}
		if c.AccountID != nil {
			row["account_id"] = *c.AccountID
		}
		if c.DisplayName != nil {
			row["display_name"] = *c.DisplayName
		}
		if c.EmailAddress != nil {
			row["email_address"] = *c.EmailAddress
		}
		cands = append(cands, row)
	}
	if len(cands) > 0 {
		out.Candidates = cands
	}
	return out, true
}

// jiraCodeForStatus derives jira-cli's normalized snake_case code from
// the HTTP status. Classification is on status, never on body text.
func jiraCodeForStatus(status int, kind ErrorType) string {
	switch status {
	case 401:
		return "jira_unauthorized"
	case 403:
		return "jira_forbidden"
	case 404:
		return "jira_not_found"
	case 409:
		return "jira_conflict"
	case 410:
		return "jira_gone"
	case 429:
		return "jira_rate_limited"
	case 400:
		return "jira_bad_request"
	default:
		if status >= 500 {
			return "jira_server_error"
		}
		return defaultCodeForType(kind)
	}
}

func jiraHintForStatus(status int, kind ErrorType) string {
	switch {
	case status == 401:
		return "Check the profile credential with `jira auth status`."
	case status == 403:
		return "The credential lacks permission for this resource."
	case status == 404:
		return "Verify the issue key or resource ID exists and is visible to this account."
	case status == 429:
		return "Wait for the retry window, then retry."
	case status >= 500:
		return "Jira reported a server-side error; retry after a short backoff."
	case kind == ErrorTypeValidation:
		return "Review the request fields and retry."
	default:
		return ""
	}
}

// classifyUntyped is the legacy substring classifier for bare errors
// that carry no typed metadata. It is the fallback only — typed errors
// must not reach it.
func classifyUntyped(err error) Error {
	msg := err.Error()
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(lower, "unsupported auth type"):
		return NewError(ErrorTypeValidation, msg)
	case strings.Contains(lower, "credential") || strings.Contains(lower, "auth"):
		return NewError(ErrorTypeAuth, msg)
	case strings.Contains(lower, "not found"):
		return NewError(ErrorTypeNotFound, msg)
	case strings.Contains(lower, "rate limit") || strings.Contains(lower, "too many"):
		return NewError(ErrorTypeRateLimit, msg)
	case strings.Contains(lower, "server"):
		return NewError(ErrorTypeServer, msg)
	default:
		return NewError(ErrorTypeValidation, msg)
	}
}

// ErrorEnvelope builds a failure envelope: ok:false, data:null,
// meta.exit_code set, and a single structured error. Machine envelopes
// omit meta.profile entirely.
func ErrorEnvelope(command string, err error) Envelope {
	cliErr := MapError(err)
	exit := ExitCode(cliErr)
	return Envelope{
		OK: false,
		Meta: Meta{
			Command:   command,
			ExitCode:  &exit,
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			RequestID: NewRequestID(),
		},
		Data:     nil,
		Errors:   []Error{cliErr},
		Warnings: []Warning{},
	}
}
