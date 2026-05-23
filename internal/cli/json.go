package cli

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"time"

	"github.com/gechr/clog"
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
type Meta struct {
	Command    string      `json:"command"`
	ExitCode   *int        `json:"exit_code,omitempty"`
	Timestamp  string      `json:"timestamp"`
	RequestID  string      `json:"request_id,omitempty"`
	Pagination *Pagination `json:"pagination,omitempty"`
}

// Pagination is the JSON envelope's output-shape descriptor for paginated
// responses. The startAt/nextPageToken fields here describe the SHAPE the
// CLI emits — they are not cursor-management state in command code.
// pagination-exempt: output-shape only, not consumer-side cursor management.
type Pagination struct {
	StartAt    int    `json:"startAt"` //nolint:revive // pagination-exempt
	MaxResults int    `json:"maxResults"`
	Total      int    `json:"total"`
	IsLast     bool   `json:"isLast"`
	NextCursor string `json:"nextCursor,omitempty"`
}

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

	// Flag / Field / Path scope a validation failure to an input.
	Flag  string `json:"flag,omitempty"`
	Field string `json:"field,omitempty"`
	Path  string `json:"path,omitempty"`

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
	ew := &errWriter{w: w}
	clog.New(clog.NewOutput(ew, clog.ColorNever)).Print().Mode(clog.JSONFlat).JSON(env)
	return ew.err
}

// WriteCompact serializes the JSON data payload to w without the
// envelope wrapper. A clog encode or write failure is surfaced to the
// caller rather than silently dropped.
func WriteCompact(w io.Writer, data any) error {
	ew := &errWriter{w: w}
	clog.New(clog.NewOutput(ew, clog.ColorNever)).Print().Mode(clog.JSONFlat).JSON(data)
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
