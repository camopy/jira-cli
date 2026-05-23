// Package cmdutil holds the cross-cutting helper layer shared by every
// jira-cli command: envelope writers, client/profile accessors, output-mode
// resolution, mutation gates, the credential-warning sink, and small generic
// value helpers. It is a strict leaf package — it depends on the shared
// internal/cli, internal/config, internal/jira, and internal/adf layers but
// never on cmd/jira or any internal/cli/<command> package.
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

// NoInputRequested reports whether the caller opted out of interactive
// prompts via the root --no-input flag. It is the single source of truth
// for headless-mode detection across every subcommand.
//
// Commands MUST read root persistent flags through this helper rather than
// re-declaring a same-name local flag: a local flag of the same name shadows
// the inherited one, so `jira --no-input issue create` would set the root
// flag while the handler read an unset local copy.
func NoInputRequested(cmd *cobra.Command) bool {
	v, _ := cmd.Root().PersistentFlags().GetBool("no-input")
	return v
}
