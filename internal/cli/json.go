package cli

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"time"

	"github.com/gechr/clog"
)

type Envelope struct {
	Meta     Meta      `json:"meta"`
	Data     any       `json:"data"`
	Errors   []Error   `json:"errors"`
	Warnings []Warning `json:"warnings"`
}

type Meta struct {
	Command    string      `json:"command"`
	Profile    string      `json:"profile"`
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

type Error struct {
	Type    string `json:"type"`
	Message string `json:"message"`
	// Candidates carries structured disambiguation context — currently
	// populated by the watcher resolver when /user/search returns 2+
	// matches (per envelope-shapes.md `errors[].candidates`). Empty
	// elsewhere; the omitempty tag keeps the wire shape minimal for
	// non-ambiguity error paths.
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

func WriteEnvelope(w io.Writer, env Envelope) error {
	clog.New(clog.NewOutput(w, clog.ColorAuto)).Print().JSON(env)
	return nil
}

func WriteCompact(w io.Writer, data any) error {
	clog.New(clog.NewOutput(w, clog.ColorAuto)).Print().Mode(clog.JSONFlat).JSON(data)
	return nil
}

func NewRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err == nil {
		return hex.EncodeToString(b[:])
	}
	return fmt.Sprintf("%d", time.Now().UnixNano())
}
