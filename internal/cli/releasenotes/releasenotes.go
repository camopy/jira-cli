package releasenotes

import (
	"fmt"
	"strings"

	clib "github.com/gechr/clib/cli/cobra"
	xslices "github.com/gechr/x/slices"
	changelog "github.com/matcra587/jira-cli"
	"github.com/matcra587/jira-cli/internal/cli"
	"github.com/matcra587/jira-cli/internal/cli/cmdutil"
	"github.com/spf13/cobra"
)

type options struct {
	latest  bool
	noPager bool
}

// NewCommand returns the `release-notes` command.
func NewCommand() *cobra.Command {
	var opts options
	cmd := &cobra.Command{
		Use:     "release-notes [version]",
		Aliases: []string{"rn"},
		Short:   "Show what's changed in jira-cli",
		Long: "Print jira-cli's own release notes — embedded in the binary — so you can " +
			"see what changed in the tool without opening GitHub or the docs site.\n\n" +
			"With no arguments it prints the full changelog, newest first. Pass a version " +
			"(e.g. `0.7.7`) for a single release, or `--latest` for just the newest one. " +
			"Human output is rendered Markdown; `--output=json` returns the notes as " +
			"structured data.\n\n" +
			"On an interactive terminal, a full history taller than the screen pages " +
			"like `git log` (`JIRA_PAGER`/`PAGER` selects an external pager; a built-in " +
			"one is the default). Pass `--no-pager` to print directly. Piped output, " +
			"machine modes, and agent sessions always stream straight through and " +
			"never page.\n\n" +
			"This is jira-cli's changelog. It is unrelated to a Jira project's releases.",
		Example: `$ jira release-notes
$ jira release-notes --latest
$ jira rn 0.7.7
$ jira release-notes --output=json`,
		GroupID: "configuration",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(cmd, opts, args)
		},
	}
	cmdutil.AddBoolVar(cmd.Flags(), &opts.latest, "latest", false, "Show only the newest release", clib.FlagExtra{})
	cmdutil.AddBoolVar(cmd.Flags(), &opts.noPager, "no-pager", false, "Print directly instead of paging long output", clib.FlagExtra{Group: "Output"})
	return cmd
}

func run(cmd *cobra.Command, opts options, args []string) error {
	var query string
	if len(args) == 1 {
		query = strings.TrimSpace(args[0])
	}
	if query != "" && opts.latest {
		return fmt.Errorf("validation: pass a version or --latest, not both")
	}

	result, err := buildResult(query, opts.latest)
	if err != nil {
		return err
	}
	return cmdutil.WriteEnvelope(cmd, "release.notes", result)
}

// buildResult selects which notes to return: a single version when one is named,
// the newest with --latest, or the whole changelog by default.
func buildResult(query string, latest bool) (cli.ReleaseNotesResult, error) {
	switch {
	case query != "":
		r, ok := changelog.Find(query)
		if !ok {
			return cli.ReleaseNotesResult{}, fmt.Errorf("no release notes for %q; available: %s", query, strings.Join(versionList(), ", "))
		}
		return single(r), nil
	case latest:
		releases := changelog.Releases()
		if len(releases) == 0 {
			return cli.ReleaseNotesResult{}, fmt.Errorf("no release notes are embedded in this build")
		}
		return single(releases[0]), nil
	default:
		return cli.ReleaseNotesResult{
			Releases: changelog.Releases(),
			Markdown: changelog.Full(),
		}, nil
	}
}

// single wraps one release as a result whose Markdown is just that release.
func single(r changelog.Release) cli.ReleaseNotesResult {
	return cli.ReleaseNotesResult{
		Releases: []changelog.Release{r},
		Markdown: r.Markdown,
	}
}

// versionList is the available versions, newest first, for error messages.
func versionList() []string {
	releases := changelog.Releases()
	return xslices.Map(releases, func(r changelog.Release) string { return r.Version })
}
