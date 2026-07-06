package cli

import (
	"context"
	"errors"
	"strings"
	"time"

	xstrings "github.com/gechr/x/strings"
	"github.com/matcra587/jira-cli/internal/adf"
	"github.com/matcra587/jira-cli/internal/config"
	"github.com/matcra587/jira-cli/internal/issuekey"
	"github.com/matcra587/jira-cli/internal/jira"
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
// the type and that code's canonical hint. Callers with a more specific
// code should set Error.Code AND Error.Hint directly afterward — every
// code carries a non-empty hint (the taxonomy guard enforces it).
func NewError(kind ErrorType, msg string) Error {
	code := defaultCodeForType(kind)
	return Error{Type: string(kind), Code: code, Message: msg, Hint: defaultHintForCode(code), Retryable: kind == ErrorTypeRateLimit}
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

// defaultHintForCode is the canonical next-action hint for each fallback
// code, so even an untyped error hands the caller a next step. Specific
// codes carry their own hints at the site that sets the code.
func defaultHintForCode(code string) string {
	switch code {
	case "auth_failed":
		return "Authentication failed — check the credential with `jira auth status`, then re-login with `jira auth login`."
	case "not_found":
		return "Re-resolve the identifier; it does not exist or is not visible to this account."
	case "validation_failed":
		return "Review the request fields and retry."
	case "rate_limited":
		return "Raise --max-retry-wait (or JIRA_MAX_RETRY_WAIT) to wait out longer limits, or retry once the window resets."
	case "server_error":
		return "Jira reported an unexpected error; retry after a short backoff."
	default:
		return "Rerun with --debug and report the failure if it persists."
	}
}

func ExitCode(err Error) int {
	// canceled and timeout carry their own exit codes (6, 7) so a caller
	// can distinguish a cancellation or deadline from a real backend
	// failure (5). They are emitted with ErrorTypeServer, so this Code
	// branch must precede the type switch. The codes are set only by
	// mapContextError; prompt cancellation uses prompt_canceled and is
	// unaffected.
	switch err.Code {
	case "canceled":
		return 6
	case "timeout":
		return 7
	}
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
// carries a stable code, a next-action hint, and a retryable flag.
// *config.CredentialError satisfies this.
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
	if out, ok := mapCLIInputError(err); ok {
		return out
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
	if out, ok := mapProfileError(err); ok {
		return out
	}
	if out, ok := mapLossyConversionError(err); ok {
		return out
	}
	if out, ok := mapADFInvalidError(err); ok {
		return out
	}
	if out, ok := mapReadOnlyError(err); ok {
		return out
	}
	if out, ok := mapDryRunBlockedError(err); ok {
		return out
	}
	if out, ok := mapJiraAPIError(err); ok {
		return out
	}
	if out, ok := mapIssueKeyError(err); ok {
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

// mapCLIInputError adapts a *CLIInputError — a bad flag, a missing
// required flag, the wrong positional-argument count, or an unknown
// command. Every such failure is an exit-3 validation error; the typed
// error supplies the specific code and remediation hint, and the offending
// flag name when the failure is flag-scoped. Recognizing it here via
// errors.As keeps it off the substring classifier, where "unknown flag"
// or "unknown command" carries no stable code.
func mapCLIInputError(err error) (Error, bool) {
	var ie *CLIInputError
	if !errors.As(err, &ie) {
		return Error{}, false
	}
	out := NewError(ErrorTypeValidation, ie.Message)
	out.Code = ie.code()
	out.Hint = ie.hint()
	out.Flag = ie.Flag
	return out, true
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

// mapCredentialError adapts a *config.CredentialError. The
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

// mapProfileError adapts the typed profile-resolution failures: a profile
// that is not defined, or one defined without a base URL. Both map to the
// contract's exit-2 not-found class with the stable profile_not_found code,
// so an agent's typoed or unprovisioned --profile fails closed instead of
// degrading into fabricated empty results. Recognizing them here via
// errors.As keeps them off the substring classifier, where "is not defined"
// carries no stable code.
func mapProfileError(err error) (Error, bool) {
	var notDefined config.ProfileNotDefinedError
	if errors.As(err, &notDefined) {
		out := NewError(ErrorTypeNotFound, notDefined.Error())
		out.Code = "profile_not_found"
		out.Hint = "List profiles with `jira config profile`; create one with `jira auth login --profile <name>`."
		return out, true
	}
	var incomplete config.ProfileIncompleteError
	if errors.As(err, &incomplete) {
		out := NewError(ErrorTypeNotFound, incomplete.Error())
		out.Code = "profile_not_found"
		out.Hint = "Complete the profile with `jira auth login --profile <name>`; live commands need a base URL."
		return out, true
	}
	return Error{}, false
}

// mapLossyConversionError adapts the strict-mode Markdown→ADF abort. The
// message is already source-mapped (line/column and the offending snippet);
// the mapper's job is the stable code and a never-empty remediation hint.
func mapLossyConversionError(err error) (Error, bool) {
	var lossy adf.LossyConversionError
	if !errors.As(err, &lossy) {
		return Error{}, false
	}
	out := NewError(ErrorTypeValidation, lossy.Error())
	out.Code = "markdown_lossy_conversion"
	out.Hint = "Rewrite the named Markdown construct (see `jira agent guide adf_reference`), " +
		"or pass `--adf-best-effort` to accept the documented downgrade."
	return out, true
}

// mapADFInvalidError adapts an *adf.InvalidDocumentError — a payload value
// of the wrong JSON shape where an ADF document object belongs. The typed
// error's clean message replaces the raw json unmarshal text, which must
// never reach the envelope; `field` names the offending payload key when
// the decode path could tell.
func mapADFInvalidError(err error) (Error, bool) {
	var invalid *adf.InvalidDocumentError
	if !errors.As(err, &invalid) {
		return Error{}, false
	}
	out := NewError(ErrorTypeValidation, invalid.Error())
	out.Code = "adf_invalid"
	out.Hint = "This field must be an ADF document (object), not a " + invalid.Got +
		"; see `jira agent guide adf_reference` for the document shape, or use the field's *_markdown alias."
	out.Field = invalid.Field
	return out, true
}

// mapReadOnlyError adapts the transport's *jira.ReadOnlyError so a blocked
// mutation carries its own stable code instead of the validation_failed
// catch-all. Checked ahead of mapJiraAPIError: read-only refusals happen
// before any HTTP exchange, so they are not API errors.
func mapReadOnlyError(err error) (Error, bool) {
	var ro *jira.ReadOnlyError
	if !errors.As(err, &ro) {
		return Error{}, false
	}
	out := NewError(ErrorTypeValidation, ro.Error())
	out.Code = "read_only"
	out.Hint = "Read-only mode is active (JIRA_READ_ONLY or profile read_only=true); unset the env var, set the profile's read_only=false, or run the mutation under a profile that allows writes."
	return out, true
}

// mapDryRunBlockedError adapts the transport's *jira.DryRunBlockedError —
// the guard that stops a mutation reaching the wire under --dry-run.
func mapDryRunBlockedError(err error) (Error, bool) {
	var blocked *jira.DryRunBlockedError
	if !errors.As(err, &blocked) {
		return Error{}, false
	}
	out := NewError(ErrorTypeValidation, blocked.Error())
	out.Code = "dry_run_blocked"
	out.Hint = "A mutation reached the transport under --dry-run; this is an internal guard, not a user error — rerun without --dry-run to submit."
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

func mapIssueKeyError(err error) (Error, bool) {
	var limit *issuekey.ExpansionLimitError
	if !errors.As(err, &limit) {
		return Error{}, false
	}
	out := NewError(ErrorTypeValidation, limit.Error())
	out.Code = "issue_key_expansion_limit"
	out.Hint = limit.Hint()
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
		// Multiple boards matched: the caller picks one from candidates[].
		// Candidate-less board failures (no default board set) keep the
		// validation_failed catch-all and its canonical hint.
		out.Code = "board_ambiguous"
		out.Hint = "Pick a board from candidates[] and pass it via --board, or set a default board for the profile."
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
	out.Code = "user_ambiguous"
	out.Hint = "Pick an account_id from candidates[] and pass it directly instead of the email."
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
//
// jiraCodeForStatus and jiraHintForStatus move in lockstep: a status
// added to one MUST be added to the other and to
// TestEveryMappedJiraStatusCarriesAHint — a jira_* code with no hint is
// the exact gap that guard exists to catch.
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
	case status == 400:
		return "Jira rejected the request — check the upstream_messages and upstream_field_errors fields for the specifics, then correct the input before resubmitting."
	case status == 401:
		return "Check the profile credential with `jira auth status`."
	case status == 403:
		return "The credential authenticates but lacks permission here — run `jira auth status` to confirm the active profile, then check the project role or global permission (or token scope) in Jira."
	case status == 404:
		return "Verify the issue key or resource ID exists and is visible to this account."
	case status == 409:
		return "The resource changed since it was read — re-fetch the issue, then retry the operation against the latest version."
	case status == 410:
		return "The resource was permanently deleted from Jira and cannot be restored — drop any cached reference to it."
	case status == 429:
		return "Jira rate-limited the request beyond the retry budget. Raise --max-retry-wait (or JIRA_MAX_RETRY_WAIT) to wait out longer limits, or retry once the window resets."
	case status >= 500:
		return "Jira reported a server-side error; retry after a short backoff."
	default:
		// An unmapped status falls back to the type's canonical hint, so a
		// jira-mapped error never ships hintless.
		return defaultHintForCode(defaultCodeForType(kind))
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
	case xstrings.ContainsAny(lower, "credential", "auth"):
		return NewError(ErrorTypeAuth, msg)
	case strings.Contains(lower, "not found"):
		return NewError(ErrorTypeNotFound, msg)
	case xstrings.ContainsAny(lower, "rate limit", "too many"):
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
