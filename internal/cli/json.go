package cli

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/gechr/clog"
	clogtheme "github.com/gechr/clog/theme"
)

// Envelope is the machine-readable JSON output contract. ok is always
// emitted; a successful command sets ok:true and a failed command sets
// ok:false with data:null and a populated errors slice.
type Envelope struct {
	OK       bool      `json:"ok"`
	Meta     Meta      `json:"meta"`
	Data     any       `json:"data"`
	Errors   []Error   `json:"errors"`
	Warnings []Warning `json:"warnings"`
}

// Meta is the envelope's command-context block. Machine envelopes omit
// profile entirely — a command that must report a profile puts it in
// command-specific data. ExitCode is present only on failure envelopes.
//
// RequestID is a locally generated correlation id; UpstreamRequestID is
// Jira's own trace id (Atl-Traceid / X-ARequestId) for the exchange —
// the value to quote to Atlassian support — present whenever the command
// had a Jira response to read it from.
type Meta struct {
	Command           string      `json:"command"`
	ExitCode          *int        `json:"exit_code,omitempty"`
	Timestamp         string      `json:"timestamp"`
	RequestID         string      `json:"request_id,omitempty"`
	UpstreamRequestID string      `json:"upstream_request_id,omitempty"`
	Pagination        *Pagination `json:"pagination,omitempty"`
}

// Pagination is the JSON envelope's output-shape descriptor for paginated
// responses. The startAt/nextPageToken fields here describe the SHAPE the
// CLI emits — they are not cursor-management state in command code.
// pagination-exempt: output-shape only, not consumer-side cursor management.
//
// This is the ONE pagination shape and it lives in ONE place: meta.pagination
// on single-target reads, and results[].data.pagination (same shape) inside
// keyed multi-key results. isLast and nextCursor are the authoritative walk
// signals; total appears only when the endpoint reports a real total —
// token-paged endpoints (enhanced search) never do, so a fabricated 0 is
// never emitted.
type Pagination struct {
	StartAt    int    `json:"startAt"` //nolint:revive // pagination-exempt
	MaxResults int    `json:"maxResults"`
	Total      *int   `json:"total,omitempty"`
	IsLast     bool   `json:"isLast"`
	NextCursor string `json:"nextCursor,omitempty"`
}

// KnownTotal wraps an authoritative total for Pagination.Total. Only call
// it with a value the endpoint actually reported.
func KnownTotal(total int) *int { return &total }

// Error is one structured failure entry in the envelope errors slice.
// type, code, message, hint, and retryable are always present; agents
// branch on code (stable snake_case), never on message. The remaining
// fields are optional context, omitted when empty.
//
// message and hint are disjoint. message states what failed — a
// diagnosis, not a wording contract; it may carry summarized upstream
// text. hint states what to do next — remediation; it may name
// commands, fields, or retry behavior. Remediation never belongs in
// message, and a diagnosis never belongs in hint.
type Error struct {
	Type      string `json:"type"`
	Code      string `json:"code"`
	Message   string `json:"message"`
	Hint      string `json:"hint"`
	Retryable bool   `json:"retryable"`

	// Flag / Field / Path scope a validation failure to an input;
	// Suggestions carries "did you mean" candidates for it, pre-formatted
	// as the caller would type them (e.g. "--output").
	Flag        string   `json:"flag,omitempty"`
	Field       string   `json:"field,omitempty"`
	Path        string   `json:"path,omitempty"`
	Suggestions []string `json:"suggestions,omitempty"`

	// HTTPStatus / RetryAfterSeconds carry transport metadata.
	HTTPStatus         int `json:"http_status,omitempty"`
	RetryAfterSeconds  int `json:"retry_after_seconds,omitempty"`
	RateLimitRemaining int `json:"rate_limit_remaining,omitempty"`

	// Provider / UpstreamCode / UpstreamStatus preserve a backend's own
	// failure identity. UpstreamCode stays empty when the provider
	// exposes no stable machine code (Jira does not).
	Provider       string `json:"provider,omitempty"`
	UpstreamCode   string `json:"upstream_code,omitempty"`
	UpstreamStatus int    `json:"upstream_status,omitempty"`
	// UpstreamRequestID is Jira's trace id for the failed exchange
	// (Atl-Traceid / X-ARequestId) — the value Atlassian support can
	// correlate, unlike the locally generated meta.request_id.
	UpstreamRequestID string `json:"upstream_request_id,omitempty"`

	// UpstreamMessages / UpstreamFieldErrors preserve Jira's
	// ErrorCollection body verbatim. Wording is not contractual; these
	// are diagnostic context, never a branch target.
	UpstreamMessages    []string          `json:"upstream_messages,omitempty"`
	UpstreamFieldErrors map[string]string `json:"upstream_field_errors,omitempty"`

	// Candidates carries structured disambiguation context — populated by
	// the watcher resolver when /user/search returns 2+ matches.
	Candidates []map[string]any `json:"candidates,omitempty"`
}

// Warning is a non-fatal best-effort diagnostic emitted alongside data.
// lossy is mandatory and serialized even when false; consumers branch on
// it without nil checks. field/path/node_type/mark_type are optional
// context fields.
type Warning struct {
	Type     string `json:"type"`
	Message  string `json:"message"`
	Field    string `json:"field,omitempty"`
	Path     string `json:"path,omitempty"`
	NodeType string `json:"node_type,omitempty"`
	MarkType string `json:"mark_type,omitempty"`
	Lossy    bool   `json:"lossy"`
}

// WarningSource is anything that can describe itself as a cli.Warning. The
// pkg/adf Warning satisfies this; we keep cli ignorant of pkg/adf to avoid
// the import cycle (pkg/adf -> cli -> pkg/adf via the plain renderer).
type WarningSource interface {
	WarningType() string
	WarningMessage() string
	WarningField() string
	WarningPath() string
	WarningNodeType() string
	WarningMarkType() string
	WarningIsLossy() bool
}

// WarningFrom adapts any WarningSource into a cli.Warning ready for the
// envelope. Used by command code converting pkg/adf warnings.
func WarningFrom(src WarningSource) Warning {
	return Warning{
		Type:     src.WarningType(),
		Message:  src.WarningMessage(),
		Field:    src.WarningField(),
		Path:     src.WarningPath(),
		NodeType: src.WarningNodeType(),
		MarkType: src.WarningMarkType(),
		Lossy:    src.WarningIsLossy(),
	}
}

// WriteEnvelope serializes a full JSON envelope to w. A clog encode or
// write failure is surfaced to the caller rather than silently dropped.
func WriteEnvelope(w io.Writer, env Envelope) error {
	return writeJSON(w, env, clog.JSONFlat, clog.ColorNever, nil)
}

// WriteCompact serializes the JSON data payload to w without the
// envelope wrapper, with null-valued keys dropped recursively so the
// agent-facing payload stays lean. json and human modes keep the full,
// stable schema; compact is the deliberately lossy, token-economical
// view, so in compact an absent key means the value was null. A clog
// encode or write failure is surfaced to the caller rather than silently
// dropped.
func WriteCompact(w io.Writer, data any) error {
	return writeJSON(w, stripNulls(data), clog.JSONFlat, clog.ColorNever, nil)
}

// stripNulls returns data with every null-valued map entry removed,
// recursively. It normalizes the value through encoding/json first so
// typed structs, maps, and slices are all handled the same way. Empty
// arrays and objects are kept — an empty collection is meaningful, a null
// is not. On any marshal/decode error the original data is returned
// unchanged, so compact never fails closed on a serializable payload.
func stripNulls(data any) any {
	if data == nil {
		return data
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return data
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var generic any
	if err := dec.Decode(&generic); err != nil {
		return data
	}
	return pruneNulls(generic)
}

// pruneNulls walks a generic JSON value (the shape produced by
// json.Decode into any) and deletes null-valued keys at every depth.
func pruneNulls(v any) any {
	switch t := v.(type) {
	case map[string]any:
		for k, val := range t {
			if val == nil {
				delete(t, k)
				continue
			}
			t[k] = pruneNulls(val)
		}
		return t
	case []any:
		for i, val := range t {
			t[i] = pruneNulls(val)
		}
		return t
	default:
		return v
	}
}

// WriteHumanJSON serializes JSON through clog's pretty printer for
// endpoints whose human mode still has a structured JSON contract.
// printTheme, when non-nil, retints the syntax highlighting to match the
// user's resolved theme and terminal background, so highlighted JSON stays
// readable on a light terminal just as entity colors do; nil keeps clog's
// built-in dark palette.
func WriteHumanJSON(w io.Writer, data any, printTheme *clogtheme.Theme) error {
	return writeJSON(w, data, clog.JSONPretty, clog.ColorAuto, printTheme)
}

// writeJSON encodes data through clog's JSON printer in the given mode.
//
// A note on the mode, because the names invite a dangerous mix-up:
// clog.JSONFlat here is a JSONPrintMode meaning "no indentation, one line" —
// it does NOT flatten keys. clog separately has style.JSONModeFlat, which
// dot-flattens nested keys (a -> {b} becomes "a.b"); jira-cli never sets it,
// so the envelope's key structure is always preserved verbatim. Don't conflate
// the two: enabling key flattening would silently reshape the machine envelope
// that agents and the slack-cli sibling depend on.
func writeJSON(w io.Writer, data any, mode clog.JSONPrintMode, color clog.ColorMode, printTheme *clogtheme.Theme) error {
	ew := &errWriter{w: w}
	out := io.Writer(ew)
	if _, ok := w.(interface{ Fd() uintptr }); ok {
		out = fdErrFile{errWriter: ew}
	}
	logger := clog.New(clog.NewOutput(out, color))
	if printTheme != nil {
		logger.SetTheme(clogtheme.Single(printTheme))
	}
	logger.Print().Mode(mode).JSON(data)
	return ew.err
}

// errWriter wraps an io.Writer and captures the first write error so a
// clog Print() call that ignores its own write result still surfaces a
// broken-pipe or quota failure to the envelope caller.
type errWriter struct {
	w   io.Writer
	err error
}

func (e *errWriter) Write(p []byte) (int, error) {
	if e.err != nil {
		return 0, e.err
	}
	n, err := e.w.Write(p)
	if err != nil {
		e.err = err
	}
	return n, err
}

type fdErrFile struct {
	*errWriter
}

func (e fdErrFile) Fd() uintptr {
	return e.w.(interface{ Fd() uintptr }).Fd()
}

func (e fdErrFile) Read(p []byte) (int, error) {
	if r, ok := e.w.(io.Reader); ok {
		return r.Read(p)
	}
	return 0, os.ErrInvalid
}

func (e fdErrFile) Close() error {
	return nil
}

// NewRequestID returns a 32-character hex request id. crypto/rand is the
// source; on the practically-impossible read failure it falls back to a
// timestamp seed, still hex-encoded and zero-padded so the id keeps its
// fixed 32-char shape for any consumer that parses it.
func NewRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err == nil {
		return hex.EncodeToString(b[:])
	}
	return fmt.Sprintf("%032x", time.Now().UnixNano())
}
