package main

import (
	"github.com/spf13/cobra"
)

// Canonical accessors for inherited root state. Commands MUST read root
// persistent flags through these helpers rather than re-declaring a
// same-name local flag: a local flag of the same name shadows the
// inherited one, so `jira --no-input issue create` would set the root
// flag while the handler read an unset local copy.

// noInputRequested reports whether the caller opted out of interactive
// prompts via the root `--no-input` flag. It is the single source of
// truth for headless-mode detection across every subcommand.
func noInputRequested(cmd *cobra.Command) bool {
	v, _ := cmd.Root().PersistentFlags().GetBool("no-input")
	return v
}
