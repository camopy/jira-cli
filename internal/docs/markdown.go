// Package docs renders a Cobra command tree to byte-stable Markdown for the
// jira-cli documentation site. It mirrors the GitHub CLI's internal/docs
// generator: GenMarkdownTreeCustom walks the tree and GenMarkdownCustom renders
// one page per command, taking a filePrepender for site front-matter and a
// linkHandler for relative links between pages. Output is deterministic — no
// generation timestamp, stable ordering — so a CI drift gate only ever sees
// real changes.
package docs

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// basenameAnnotation lets a command override its generated file basename
// (default: the command path with spaces replaced by underscores).
const basenameAnnotation = "markdown:basename"

// GenMarkdownTreeCustom walks cmd and every descendant, writing one Markdown
// page per command into dir. filePrepender receives the path of the file about
// to be written and returns content prepended to it (site front-matter);
// callers embedding it in output should pass it through filepath.Base so a
// machine-specific path can't leak into output and break determinism.
// linkHandler rewrites a target page basename into a site-relative link. It is
// the entry point used by cmd/gen-docs.
func GenMarkdownTreeCustom(cmd *cobra.Command, dir string, filePrepender, linkHandler func(string) string) (err error) {
	for _, c := range cmd.Commands() {
		if !c.IsAvailableCommand() || c.IsAdditionalHelpTopicCommand() {
			continue
		}
		if err = GenMarkdownTreeCustom(c, dir, filePrepender, linkHandler); err != nil {
			return err
		}
	}
	base := commandBasename(cmd)
	path := filepath.Join(dir, base+".md")
	var f *os.File
	f, err = os.Create(path)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()
	if _, err = io.WriteString(f, filePrepender(path)); err != nil {
		return err
	}
	return GenMarkdownCustom(cmd, f, filePrepender, linkHandler)
}

// commandBasename returns the file basename for cmd: the markdown:basename
// annotation if set, else the full command path with spaces as underscores
// (e.g. "jira issue create" -> "jira_issue_create"). The annotation value is
// passed through filepath.Base so it cannot contain a directory component and
// escape the output directory when joined into a path.
func commandBasename(cmd *cobra.Command) string {
	if v, ok := cmd.Annotations[basenameAnnotation]; ok && v != "" {
		return filepath.Base(v)
	}
	return strings.ReplaceAll(cmd.CommandPath(), " ", "_")
}

// GenMarkdownCustom renders a single command's page to w. linkHandler rewrites
// references to other generated pages; filePrepender is accepted for signature
// parity with the tree walker and is not used per-command.
func GenMarkdownCustom(cmd *cobra.Command, w io.Writer, _, linkHandler func(string) string) error {
	cmd.InitDefaultHelpFlag()
	cmd.InitDefaultVersionFlag()

	var b bytes.Buffer
	name := cmd.CommandPath()
	fmt.Fprintf(&b, "# %s\n\n", name)
	if cmd.Short != "" {
		fmt.Fprintf(&b, "%s\n\n", cmd.Short)
	}
	if cmd.Long != "" {
		fmt.Fprintf(&b, "%s\n\n", cmd.Long)
	}
	if cmd.Runnable() {
		fmt.Fprintf(&b, "```\n%s\n```\n\n", cmd.UseLine())
	}

	// Subcommands are sorted by name once and reused for the SEE ALSO section
	// below, keeping the render to a single sort.
	subs := availableSubcommands(cmd)

	// Subcommand links.
	if len(subs) > 0 {
		fmt.Fprintf(&b, "### Subcommands\n\n")
		for _, c := range subs {
			link := linkHandler(commandBasename(c) + ".md")
			fmt.Fprintf(&b, "* [%s](%s)\t - %s\n", c.CommandPath(), link, c.Short)
		}
		fmt.Fprintln(&b)
	}

	// Options (FlagUsages is already alpha-sorted by cobra).
	if flags := cmd.NonInheritedFlags(); flags.HasAvailableFlags() {
		fmt.Fprintf(&b, "### Options\n\n```\n%s```\n\n", flags.FlagUsages())
	}
	if flags := cmd.InheritedFlags(); flags.HasAvailableFlags() {
		fmt.Fprintf(&b, "### Options inherited from parent commands\n\n```\n%s```\n\n", flags.FlagUsages())
	}

	if cmd.Example != "" {
		fmt.Fprintf(&b, "### Examples\n\n```\n%s\n```\n\n", cmd.Example)
	}

	// SEE ALSO: parent, then sorted subcommands.
	if cmd.HasParent() || len(subs) > 0 {
		fmt.Fprintf(&b, "### SEE ALSO\n\n")
		if cmd.HasParent() {
			p := cmd.Parent()
			fmt.Fprintf(&b, "* [%s](%s)\t - %s\n", p.CommandPath(), linkHandler(commandBasename(p)+".md"), p.Short)
		}
		for _, c := range subs {
			fmt.Fprintf(&b, "* [%s](%s)\t - %s\n", c.CommandPath(), linkHandler(commandBasename(c)+".md"), c.Short)
		}
		fmt.Fprintln(&b)
	}

	_, err := w.Write(b.Bytes())
	return err
}

// availableSubcommands returns cmd's visible subcommands sorted by name, for
// deterministic output.
func availableSubcommands(cmd *cobra.Command) []*cobra.Command {
	var out []*cobra.Command
	for _, c := range cmd.Commands() {
		if c.IsAvailableCommand() && !c.IsAdditionalHelpTopicCommand() {
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	return out
}
