package cmdutil

import (
	"fmt"
	"io"
	"strings"

	clib "github.com/gechr/clib/cli/cobra"
	stdininput "github.com/matcra587/jira-cli/internal/cli/stdin"
	"github.com/spf13/cobra"
)

// AddMarkdownFlag registers the canonical Markdown body bundle bound to p
// and file: `--markdown` takes the body inline, `--markdown-file` reads it
// from a file (- reads stdin) for multi-paragraph content that shell
// quoting would mangle. One spelling pair across every mutating command,
// replacing the per-command field-named variants.
//
// When alias is non-empty it is registered as a hidden alias under the
// command's historical flag name (`--body-markdown`, `--comment-markdown`,
// `--description-markdown`) so existing scripts and agent prompts keep
// working. The alias shares p — the mutual exclusions below guarantee at
// most one spelling is set per invocation — and stays out of help, the
// agent schema, and the generated reference, all of which skip hidden
// flags. It is hidden rather than deprecated because pflag routes its
// deprecation notice through the command's output stream, which would
// break the byte-clean-stdout envelope contract for the very agents still
// using the old spelling.
//
// Every body source is mutually exclusive with `--json-input`: they are
// different ways to set the same field, so the caller picks one. This
// replaces the earlier silent flag-over-payload precedence with a clear
// error.
func AddMarkdownFlag(cmd *cobra.Command, p, file *string, usage, alias string) {
	AddStringVar(cmd.Flags(), p, "markdown", "", usage, clib.FlagExtra{Group: "Input", Placeholder: "MARKDOWN"})
	AddFileFlag(cmd.Flags(), file, "markdown-file", "", "Read the Markdown body from a file; - reads stdin", "Input", "FILE")
	cmd.MarkFlagsMutuallyExclusive("markdown", "json-input")
	cmd.MarkFlagsMutuallyExclusive("markdown-file", "markdown")
	cmd.MarkFlagsMutuallyExclusive("markdown-file", "json-input")
	if alias == "" {
		return
	}
	AddStringVar(cmd.Flags(), p, alias, "", usage, clib.FlagExtra{Group: "Input", Placeholder: "MARKDOWN"})
	_ = cmd.Flags().MarkHidden(alias)
	cmd.MarkFlagsMutuallyExclusive("markdown", alias)
	cmd.MarkFlagsMutuallyExclusive(alias, "json-input")
	cmd.MarkFlagsMutuallyExclusive(alias, "markdown-file")
}

// ResolveMarkdownInput returns the Markdown body from the AddMarkdownFlag
// bundle: the inline value (--markdown or its alias, which share a
// variable), or the content of --markdown-file with - meaning stdin.
// Mutual exclusion guarantees at most one source is set.
func ResolveMarkdownInput(inline, file string) (string, error) {
	path := strings.TrimSpace(file)
	if path == "" {
		return inline, nil
	}
	r, err := stdininput.TextInput(path)
	if err != nil {
		return "", fmt.Errorf("read --markdown-file: %w", err)
	}
	defer r.Close() //nolint:errcheck // read-side close; the content is already in hand
	data, err := io.ReadAll(r)
	if err != nil {
		return "", fmt.Errorf("read --markdown-file: %w", err)
	}
	return string(data), nil
}

// MarkdownFlagChanged reports whether any Markdown body source — the
// canonical --markdown flag, --markdown-file, or, when alias is non-empty,
// the hidden alias — was set on cmd.
func MarkdownFlagChanged(cmd *cobra.Command, alias string) bool {
	if cmd == nil {
		return false
	}
	if cmd.Flags().Changed("markdown") || cmd.Flags().Changed("markdown-file") {
		return true
	}
	return alias != "" && cmd.Flags().Changed(alias)
}
