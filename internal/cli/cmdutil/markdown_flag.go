package cmdutil

import (
	clib "github.com/gechr/clib/cli/cobra"
	"github.com/spf13/cobra"
)

// AddMarkdownFlag registers the canonical `--markdown` flag bound to p: one
// spelling for "supply this command's rich-text field as Markdown" across
// every mutating command, replacing the per-command field-named variants.
//
// When alias is non-empty it is registered as a hidden deprecated alias
// under the command's historical flag name (`--body-markdown`,
// `--comment-markdown`, `--description-markdown`) so existing scripts and
// agent prompts keep working. The alias shares p — the mutual exclusion
// below guarantees at most one spelling is set per invocation — and stays
// out of help, the agent schema, and the generated reference, all of which
// skip hidden flags.
//
// Both spellings are mutually exclusive with `--json-input`: they are two
// ways to set the same field, so the caller must pick one. This replaces
// the earlier silent flag-over-payload precedence with a clear error.
func AddMarkdownFlag(cmd *cobra.Command, p *string, usage, alias string) {
	AddStringVar(cmd.Flags(), p, "markdown", "", usage, clib.FlagExtra{Group: "Input", Placeholder: "MARKDOWN"})
	cmd.MarkFlagsMutuallyExclusive("markdown", "json-input")
	if alias == "" {
		return
	}
	AddStringVar(cmd.Flags(), p, alias, "", usage, clib.FlagExtra{Group: "Input", Placeholder: "MARKDOWN"})
	// Hidden, not deprecated: pflag routes the deprecation notice through
	// the command's output stream, which would break the byte-clean-stdout
	// envelope contract for agents still passing the old spelling. The
	// alias simply works, silently.
	_ = cmd.Flags().MarkHidden(alias)
	cmd.MarkFlagsMutuallyExclusive("markdown", alias)
	cmd.MarkFlagsMutuallyExclusive(alias, "json-input")
}

// MarkdownFlagChanged reports whether the canonical --markdown flag — or,
// when alias is non-empty, its hidden deprecated alias — was set on cmd.
func MarkdownFlagChanged(cmd *cobra.Command, alias string) bool {
	if cmd == nil {
		return false
	}
	if cmd.Flags().Changed("markdown") {
		return true
	}
	return alias != "" && cmd.Flags().Changed(alias)
}
