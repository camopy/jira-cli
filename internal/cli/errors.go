package cli

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"time"

	xstrings "github.com/gechr/x/strings"
	"github.com/matcra587/jira-cli/internal/adf"
	"github.com/matcra587/jira-cli/internal/config"
	"github.com/matcra587/jira-cli/internal/errtax"
	"github.com/matcra587/jira-cli/internal/jira"
)

// ErrorType aliases the shared taxonomy type; the historical cli names
// remain for the command layer.
type ErrorType = errtax.Type

// The taxonomy types, re-exported under cli's historical names.
const (
	ErrorTypeAuth       = errtax.TypeAuth
	ErrorTypeNotFound   = errtax.TypeNotFound
	ErrorTypeValidation = errtax.TypeValidation
	ErrorTypeRateLimit  = errtax.TypeRateLimit
	ErrorTypeServer     = errtax.TypeServer
)

// newFromCode builds the envelope Error for a code, deriving type, hint,
// and retryability from the errtax registry. An unregistered code fails
// closed — server type, the generic hint, retryable false — so the
// envelope never ships an empty type or hint.
func newFromCode(code errtax.Code, msg string) Error {
	spec, ok := errtax.Lookup(code)
	if !ok {
		return Error{
			Type:    string(errtax.TypeServer),
			Code:    string(code),
			Message: msg,
			Hint:    errtax.HintFor(errtax.CodeUnknown),
		}
	}
	return Error{
		Type:      string(spec.Type),
		Code:      string(code),
		Message:   msg,
		Hint:      spec.Hint,
		Retryable: spec.Retryable,
	}
}

// ExitCode maps a structured Error to its process exit code: a registered
// code's pinned exit (canceled=6 and timeout=7 live in the registry as
// per-code rows), else the type's default.
func ExitCode(err Error) int {
	if spec, ok := errtax.Lookup(errtax.Code(err.Code)); ok {
		return spec.Exit
	}
	return errtax.ExitFor(errtax.Type(err.Type))
}

// MapError is the central error mapper. Typed source errors describe
// themselves with an [errtax.Code]; the registry supplies everything
// derivable from it, and the capability interfaces layer per-instance
// context on top. Classification runs in tiers — never substring matching
// where a typed error exists:
//
//	tier 1: prompt identity beats a wrapped cancellation — a canceled
//	        prompt is prompt_canceled, not canceled.
//	tier 2: bare or wrapped stdlib context sentinels. errors.Is walks the
//	        chain, so a Coded error wrapping a cancellation (an APIError
//	        whose Cause is a canceled body read, a CredentialError whose
//	        Wrapped is a keyring deadline) still classifies as
//	        canceled/timeout, preserving the pre-registry adapter order.
//	tier 3: every other Coded error; the outermost one in the chain wins.
//
// A bare error falls back to a best-effort substring classifier for
// legacy untyped error strings; that tier is strictly terminal.
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
	var coded errtax.Coded
	if errors.As(err, &coded) {
		return assemble(err, coded)
	}
	return classifyUntyped(err)
}

// mapPromptError adapts a *PromptError. Every interactive-prompt
// outcome — user abort, SIGINT/timeout cancellation, or an unavailable
// prompt — is a validation-class failure (exit 3): the command could not
// gather the input it needed. It must run before mapContextError so a
// canceled prompt keeps its prompt identity rather than collapsing into
// the generic canceled mapping.
func mapPromptError(err error) (Error, bool) {
	var pe *PromptError
	if !errors.As(err, &pe) {
		return Error{}, false
	}
	return assemble(err, pe), true
}

// mapContextError adapts a context cancellation or deadline error.
// Cancellation reaches MapError when --timeout elapses or a SIGINT
// cancels the root context mid-request; classifying it here with
// errors.Is keeps it off the substring classifier, where the wrapped
// url.Error text would land in the wrong bucket. The registry rows pin
// the dedicated exits (canceled 6, timeout 7) and mark both retryable:
// the request never completed.
func mapContextError(err error) (Error, bool) {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return newFromCode(errtax.CodeTimeout, err.Error()), true
	case errors.Is(err, context.Canceled):
		return newFromCode(errtax.CodeCanceled, err.Error()), true
	default:
		return Error{}, false
	}
}

// assemble builds the envelope Error from a Coded error: the registry
// supplies type, hint, and retryability for the code, then the error's
// per-instance capabilities are layered on top.
func assemble(err error, coded errtax.Coded) Error {
	// errors.As can bind coded to a non-nil interface holding a typed-nil
	// pointer, where Code() and Error() would panic on a nil receiver.
	// Fail over to the untyped path instead.
	if isNilValue(coded) {
		return classifyUntyped(err)
	}
	out := newFromCode(coded.Code(), coded.Error())
	// Flat comma-ok probes, deliberately not a type switch: an error can
	// implement several capabilities at once, and a type switch would
	// fire only its first match.
	if f, ok := coded.(errtax.Flagger); ok {
		out.Flag = f.FlagName()
	}
	if c, ok := coded.(errtax.Candidated); ok {
		if rows := c.CandidateRows(); len(rows) > 0 {
			out.Candidates = rows
		}
	}
	// Single-implementer capabilities stay concrete assertions rather
	// than interfaces.
	if ce, ok := coded.(*config.CredentialError); ok {
		// CredentialError.Error() decorates with the type, hint, and
		// wrapped cause; the envelope keeps the bare message. Provider
		// identity stays per-instance; hint and retryability come from
		// the registry like every other code.
		out.Message = ce.Message
		if ce.Upstream != nil {
			out.Provider = ce.Upstream.Provider
			out.UpstreamCode = ce.Upstream.UpstreamCode
			out.UpstreamStatus = ce.Upstream.UpstreamStatus
		}
	}
	if inv, ok := coded.(*adf.InvalidDocumentError); ok {
		out.Field = inv.Field
	}
	if ie, ok := coded.(*CLIInputError); ok {
		// "Did you mean" candidates ride the typed suggestions field,
		// never interpolated into the static hint.
		out.Suggestions = ie.Suggestions
	}
	if ae, ok := coded.(*jira.APIError); ok {
		// APIError.Error() decorates with the type and status; the
		// envelope keeps the bare upstream message. Retryability keeps the
		// transport rule — rate-limit and server-class failures are worth
		// retrying — even where an unmapped status lands on the server
		// catch-all code (a 413, a truncated 2xx body).
		out.Message = ae.Message
		out.Retryable = out.Type == string(errtax.TypeRateLimit) || out.Type == string(errtax.TypeServer)
	}
	// Upstream Jira metadata is read through the chain, not an
	// outermost-only assertion: a Coded error wrapping an APIError would
	// otherwise silently drop the seven upstream fields.
	var api *jira.APIError
	if errors.As(err, &api) && api != nil {
		enrichAPIUpstream(&out, api)
	}
	return out
}

// enrichAPIUpstream copies the transport and ErrorCollection metadata an
// APIError carries onto the envelope entry. UpstreamCode stays empty:
// Jira exposes no stable machine-readable error code.
func enrichAPIUpstream(out *Error, apiErr *jira.APIError) {
	out.HTTPStatus = apiErr.StatusCode
	out.RetryAfterSeconds = apiErr.RetryAfterSeconds
	out.RateLimitRemaining = apiErr.RateLimitRemaining
	out.Provider = "jira"
	out.UpstreamStatus = apiErr.UpstreamStatus
	out.UpstreamMessages = apiErr.ErrorMessages
	out.UpstreamFieldErrors = apiErr.FieldErrors
}

// isNilValue reports whether the interface holds a typed-nil pointer.
func isNilValue(err error) bool {
	v := reflect.ValueOf(err)
	return v.Kind() == reflect.Pointer && v.IsNil()
}

// classifyUntyped is the legacy substring classifier for bare errors
// that carry no typed metadata. It is strictly terminal — typed errors
// must not reach it, and no new branches belong here.
func classifyUntyped(err error) Error {
	msg := err.Error()
	lower := strings.ToLower(msg)
	var kind errtax.Type
	switch {
	case strings.Contains(lower, "unsupported auth type"):
		kind = errtax.TypeValidation
	case xstrings.ContainsAny(lower, "credential", "auth"):
		kind = errtax.TypeAuth
	case strings.Contains(lower, "not found"):
		kind = errtax.TypeNotFound
	case xstrings.ContainsAny(lower, "rate limit", "too many"):
		kind = errtax.TypeRateLimit
	case strings.Contains(lower, "server"):
		kind = errtax.TypeServer
	default:
		kind = errtax.TypeValidation
	}
	return newFromCode(errtax.DefaultCode(kind), msg)
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
