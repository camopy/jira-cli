package cmdutil

import (
	"context"

	"github.com/matcra587/jira-cli/internal/cli"
	"github.com/spf13/cobra"
)

// contextKey is the unexported type for context keys owned by this package.
// Using a named type rather than a bare string prevents key collisions with
// values stored by other packages.
type contextKey string

const (
	detectorKey contextKey = "detector"
	// credentialWarnSinkKey carries the per-command credential-warning sink.
	credentialWarnSinkKey contextKey = "credential-warn-sink"
	// rateWarnSinkKey carries the per-command rate-limit-warning sink.
	rateWarnSinkKey contextKey = "rate-warn-sink"
)

// WithDetector returns a context carrying the resolved output-detection
// result. PersistentPreRunE installs one per command invocation; tests use
// it to seed a command context directly.
func WithDetector(ctx context.Context, det cli.Detection) context.Context {
	return context.WithValue(ctx, detectorKey, det)
}

// DetectorFromContext returns the output-detection result installed on the
// command context, or the zero Detection when none is present.
func DetectorFromContext(cmd *cobra.Command) cli.Detection {
	v, _ := cmd.Context().Value(detectorKey).(cli.Detection)
	return v
}

// ConfigPath returns the value of the root --config persistent flag.
func ConfigPath(cmd *cobra.Command) string {
	path, _ := cmd.Root().PersistentFlags().GetString("config")
	return path
}

// NoInputRequested reports whether interactive prompts are off. It is the
// single source of truth for headless-mode detection across every subcommand.
//
// An explicit --no-input always wins. When the flag is not set, no-input is
// implied for a non-interactive session — an agent harness or a piped/
// redirected stdin — so a scripted mutation never stalls waiting for a prompt
// it cannot show. The probe is stdin, not stdout: `jira issue edit KEY | tee`
// pipes stdout but keeps an interactive stdin, and must still open the editor.
//
// Commands MUST read root persistent flags through this helper rather than
// re-declaring a same-name local flag: a local flag of the same name shadows
// the inherited one, so `jira --no-input issue create` would set the root
// flag while the handler read an unset local copy.
func NoInputRequested(cmd *cobra.Command) bool {
	pf := cmd.Root().PersistentFlags()
	if v, _ := pf.GetBool("no-input"); v {
		return true
	}
	// The flag is false. An explicit --no-input=false wins (the caller wants
	// prompts even off a TTY); otherwise imply no-input for a non-interactive
	// session.
	if f := pf.Lookup("no-input"); f != nil && f.Changed {
		return false
	}
	det := DetectorFromContext(cmd)
	return det.Agent || det.StdinPiped
}
