package cli

import (
	"fmt"
	"io"

	"github.com/gechr/clog"
)

// RouteMode selects how RouteWarnings emits data + warnings.
type RouteMode int

const (
	// RouteJSON emits the full Envelope (data + warnings) on stdout. Stderr
	// stays untouched.
	RouteJSON RouteMode = iota
	// RoutePlain emits human-readable data on stdout and mirrors each
	// warning to stderr as a clog WRN line. Warning text MUST NOT
	// leak into stdout.
	RoutePlain
	// RouteCompact is JSON without the envelope wrapper; warnings still go
	// in the data via the caller's contract since there is no envelope.
	RouteCompact
)

// RouteOptions describes a single envelope routing call.
type RouteOptions struct {
	Stdout   io.Writer
	Stderr   io.Writer
	Mode     RouteMode
	Envelope Envelope      // used when Mode == RouteJSON
	Command  string        // used when Mode == RoutePlain
	Data     any           // used when Mode == RoutePlain
	Warnings []Warning     // used when Mode == RoutePlain (and may be empty)
	Plain    []PlainOption // optional plain renderer hints
}

// RouteWarnings is the single entry point that splits data vs warnings
// across stdout/stderr. Commands with structured warnings call this
// instead of WriteEnvelope/WriteCommandPlain directly.
func RouteWarnings(opts RouteOptions) error {
	switch opts.Mode {
	case RouteJSON:
		if err := WriteEnvelope(opts.Stdout, opts.Envelope); err != nil {
			return err
		}
		return nil
	case RouteCompact:
		return WriteCompact(opts.Stdout, opts.Data)
	case RoutePlain:
		if err := WriteCommandPlain(opts.Stdout, opts.Command, opts.Data, opts.Plain...); err != nil {
			return err
		}
		return mirrorWarningsToStderr(opts.Stderr, opts.Warnings)
	default:
		return fmt.Errorf("cli.RouteWarnings: unknown mode %d", opts.Mode)
	}
}

// mirrorWarningsToStderr emits one clog WRN line per warning, on a boundary
// logger pinned to LevelWarn — this surface is warnings-only by design.
func mirrorWarningsToStderr(w io.Writer, warnings []Warning) error {
	if len(warnings) == 0 || w == nil {
		return nil
	}
	logger := newPlainLoggerAt(w, clog.LevelWarn)
	for _, warn := range warnings {
		// Warning strings can carry Jira-controlled text — node_type/mark_type
		// echo whatever type string the inbound ADF document declared — so
		// every field crosses the terminal sanitizer at this stderr boundary.
		event := logger.Warn().Str("type", SanitizeTerminalText(warn.Type))
		if warn.Field != "" {
			event = event.Str("field", SanitizeTerminalText(warn.Field))
		}
		if warn.Path != "" {
			event = event.Str("path", SanitizeTerminalText(warn.Path))
		}
		if warn.NodeType != "" {
			event = event.Str("node_type", SanitizeTerminalText(warn.NodeType))
		}
		if warn.MarkType != "" {
			event = event.Str("mark_type", SanitizeTerminalText(warn.MarkType))
		}
		event = event.Bool("lossy", warn.Lossy)
		event.Msg(SanitizeTerminalText(warn.Message))
	}
	return nil
}
