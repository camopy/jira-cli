// Package adfcmd implements the `jira adf` command group: standalone
// Markdown→ADF conversion and linting, plus the explicitly-lossy
// ADF→Markdown projection. Both are local-only — no profile, no network —
// so an agent can pre-flight rich text in isolation and submit the
// resulting document anywhere `--json-input` is accepted.
package adfcmd

import (
	"errors"
	"fmt"
	"io"

	xstrings "github.com/gechr/x/strings"
	"github.com/spf13/cobra"

	"github.com/matcra587/jira-cli/internal/adf"
	"github.com/matcra587/jira-cli/internal/cli/adfmode"
	"github.com/matcra587/jira-cli/internal/cli/cmdutil"
	"github.com/matcra587/jira-cli/internal/cli/stdin"
)

// NewCommand returns the `adf` command group.
func NewCommand() *cobra.Command {
	cmd := cmdutil.GroupCommand("adf", "Convert and lint rich text between Markdown and ADF", "agent")
	cmd.AddCommand(convertCommand())
	cmd.AddCommand(renderCommand())
	return cmd
}

func convertCommand() *cobra.Command {
	var input string
	cmd := &cobra.Command{
		Use:   "convert",
		Short: "Convert Markdown to a native ADF document",
		Long: "Convert GFM Markdown into a native ADF document without touching Jira, through " +
			"the exact converter, normalizer, and validator the mutation pipeline runs — a " +
			"clean conversion here is a clean conversion on submit. Author rich text safely: " +
			"convert and lint in isolation, then pass the document to `--json-input` on " +
			"`jira issue create`, `jira issue edit`, `jira issue comment add`, or " +
			"`jira worklog add`.\n\n" +
			"Strict ADF mode (the mutation default) fails with exit 3 and a source-mapped " +
			"diagnostic on any lossy conversion step; `--adf-best-effort` emits the document " +
			"with the same warning taxonomy mutations use. `--output=compact` on a clean " +
			"conversion emits just the document, ready to pipe; a best-effort conversion " +
			"with warnings folds them into the compact payload, so prefer `--output=json` " +
			"there. The command is local-only and needs no profile.",
		Example: `# Convert a file and pipe the document straight into a comment
$ jira adf convert --input notes.md --output=compact | jira issue comment add PROJ-123 --json-input -

# Convert from stdin
$ cat notes.md | jira adf convert --input - --output=json

# Accept documented downgrades (bold+code, images, quotes in list items)
$ jira adf convert --input notes.md --adf-best-effort --output=json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			markdown, err := readTextInput(cmd, input)
			if err != nil {
				return err
			}
			doc, warnings, err := convertMarkdown(markdown, cmdutil.ADFModeFor(cmd, true))
			if err != nil {
				return err
			}
			return cmdutil.WriteEnvelopeWithWarnings(cmd, "adf.convert", doc, warnings)
		},
	}
	cmdutil.AddFileFlag(cmd.Flags(), &input, "input", "-", "Markdown input file; - reads stdin", "Input", "FILE")
	return cmd
}

// convertMarkdown is the pure conversion core: the same
// FromMarkdownLossy → Normalize → ValidateDoc sequence the mutation
// pipeline applies to a Markdown body, so the standalone command can never
// drift from what a submit would do. In strict mode a lossy conversion
// step aborts with the source-mapped typed error before validation.
func convertMarkdown(markdown string, mode adfmode.Mode) (adf.Document, []adf.Warning, error) {
	doc, mdWarnings, err := adf.FromMarkdownLossy(markdown)
	if err != nil {
		return adf.Document{}, nil, err
	}
	if mode == adfmode.ModeStrict {
		for _, w := range mdWarnings {
			if w.Lossy {
				return adf.Document{}, nil, adf.LossyConversionError{Warning: w}
			}
		}
	}
	normalized, normWarnings := adf.Normalize(doc)
	valWarnings, err := adf.ValidateDoc(normalized, mode)
	if err != nil {
		return adf.Document{}, nil, err
	}
	warnings := make([]adf.Warning, 0, len(mdWarnings)+len(normWarnings)+len(valWarnings))
	warnings = append(warnings, mdWarnings...)
	warnings = append(warnings, normWarnings...)
	warnings = append(warnings, valWarnings...)
	return normalized, warnings, nil
}

func renderCommand() *cobra.Command {
	var input string
	cmd := &cobra.Command{
		Use:   "render",
		Short: "Render an ADF document as Markdown (lossy)",
		Long: "Render a native ADF document as a Markdown projection for review — reading an " +
			"existing description or comment before editing it. The projection is LOSSY by " +
			"design: constructs without a Markdown spelling (mentions, panels, cards, status " +
			"lozenges) degrade or disappear, and `data.lossy_constructs` names each one. " +
			"Never convert the output back to ADF for a submit — reuse the original document " +
			"instead. Pass the document itself: `{\"body\": ...}` wrappers are not unwrapped.",
		Example: `# Review a comment body captured from comment list
$ jira adf render --input body.json --output=json

# From stdin
$ jira issue view PROJ-123 --output=compact | jq '.issue.fields.description' | jira adf render --input -`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			raw, err := readTextInput(cmd, input)
			if err != nil {
				return err
			}
			doc, warnings, err := adf.Parse([]byte(raw))
			if err != nil {
				return fmt.Errorf("validation: parse ADF input: %w", err)
			}
			res := adf.ToMarkdownLossy(doc)
			data := map[string]any{
				"markdown":         res.Markdown,
				"lossy":            len(res.LossyConstructs) > 0,
				"lossy_constructs": res.LossyConstructs,
			}
			return cmdutil.WriteEnvelopeWithWarnings(cmd, "adf.render", data, warnings)
		},
	}
	cmdutil.AddFileFlag(cmd.Flags(), &input, "input", "-", "ADF JSON input file; - reads stdin", "Input", "FILE")
	return cmd
}

// readTextInput drains the --input source (file or stdin) and rejects an
// empty payload up front with a validation-class error. The stdin default
// only reads when stdin is actually piped: a bare invocation on a TTY gets
// an immediate validation error instead of blocking on the terminal.
func readTextInput(cmd *cobra.Command, path string) (string, error) {
	if path == "-" && !cmdutil.DetectorFromContext(cmd).StdinPiped {
		return "", errors.New("validation: no piped stdin; pass --input <file> or pipe content to --input -")
	}
	rc, err := stdin.TextInput(path)
	if err != nil {
		return "", fmt.Errorf("validation: read --input: %w", err)
	}
	defer rc.Close() //nolint:errcheck // read-only stream
	data, err := io.ReadAll(rc)
	if err != nil {
		return "", fmt.Errorf("read --input: %w", err)
	}
	if xstrings.IsBlank(string(data)) {
		return "", errors.New("validation: --input is empty")
	}
	return string(data), nil
}
