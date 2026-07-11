package cli

import "github.com/gechr/clog"

// resolvedColorMode is the process-wide --color decision, published by root
// before any command renders. Package cli owns human output on stdout through
// fresh clog loggers that clog.Default (pinned to stderr) does not govern, and
// several of those surfaces — RouteWarnings, WriteHumanJSON — run without a
// *cobra.Command in reach, so the resolved mode lives here as a package global,
// mirroring clog.Default itself, rather than being threaded through every
// signature. ColorAuto (the zero value) preserves per-writer TTY/NO_COLOR
// detection, so an unset mode behaves exactly as it did before root wires it.
var resolvedColorMode = clog.ColorAuto

// SetResolvedColorMode publishes the --color decision root resolved to the
// stdout human surfaces. clog.Default carries the mode for stderr; this carries
// it for everything package cli writes to stdout.
func SetResolvedColorMode(mode clog.ColorMode) { resolvedColorMode = mode }

// ResolvedColorMode reports the currently published --color decision.
func ResolvedColorMode() clog.ColorMode { return resolvedColorMode }

// StyleEnabled reports whether ANSI styling and OSC 8 hyperlinks should be
// emitted on a human stdout surface whose raw TTY state is tty. It folds the
// resolved --color mode over TTY detection: ColorAlways forces styling even off
// a TTY, ColorNever suppresses it even on one, and ColorAuto defers to tty —
// exactly the pre-flag behavior. The decision governs styling and hyperlinks
// only; TTY-only behaviors that are not color (spinner silence, pagination)
// keep reading raw terminal detection.
func StyleEnabled(tty bool) bool {
	switch resolvedColorMode {
	case clog.ColorAlways:
		return true
	case clog.ColorNever:
		return false
	default:
		return tty
	}
}
