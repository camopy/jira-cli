package version

import (
	"fmt"
	"io"
	"time"

	clib "github.com/gechr/clib/cli/cobra"
	cliveversion "github.com/gechr/clive/version"
	"github.com/gechr/x/human"
	"github.com/spf13/cobra"

	"github.com/matcra587/jira-cli/internal/cli"
	"github.com/matcra587/jira-cli/internal/cli/cmdutil"
	"github.com/matcra587/jira-cli/internal/envelope"
	"github.com/matcra587/jira-cli/internal/version"
)

// NewCommand returns the `version` command. Human mode prints the bare
// version (or the labeled --detailed block); machine modes emit the full
// build-metadata envelope regardless of --detailed.
func NewCommand() *cobra.Command {
	var detailed bool
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Long: "Print the installed version. Human output is the bare version string; pass " +
			"`--detailed` for a labeled block with commit, branch, and build metadata. " +
			"Machine modes always carry the full metadata in the envelope, so `--detailed` " +
			"only affects human output.",
		Example: `$ jira version

# Labeled commit, branch, and build metadata
$ jira version --detailed

# Keep build metadata parseable for scripts
$ jira version --output=json`,
		GroupID: "agent",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if cmdutil.UsePlainOutput(cmd) {
				return cli.TrackWrites(cmd.OutOrStdout(), func(out io.Writer) error {
					if detailed {
						return writeDetailed(out)
					}
					_, err := fmt.Fprintln(out, cliveversion.RemovePrefix(version.Version()))
					return err
				})
			}
			return cmdutil.WriteEnvelope(cmd, "version", envelope.VersionOutput{
				Version:   version.Version(),
				Commit:    version.Commit(),
				Branch:    version.Branch,
				BuildTime: version.BuildTime(),
				BuildBy:   version.BuildBy,
				Summary:   version.String(),
			})
		},
	}
	cmdutil.AddBoolVar(cmd.Flags(), &detailed, "detailed", false,
		"Show labeled commit, branch, and build metadata",
		clib.FlagExtra{Group: "Output", Terse: "labeled build metadata"})
	return cmd
}

// writeDetailed prints the labeled metadata block:
//
//	jira v0.6.5
//	  commit:   6ce742e
//	  branch:   main
//	  built:    2 hours ago
//	  built by: goreleaser
//
// Rows whose value is unknown (e.g. no VCS info in a module-proxy build) are
// dropped rather than printed as "unknown".
func writeDetailed(w io.Writer) error {
	if _, err := fmt.Fprintf(w, "jira %s\n", version.Version()); err != nil {
		return err
	}

	type row struct {
		label, value string
	}
	rows := make([]row, 0, 4)
	add := func(label, value string) {
		if value != "" && value != "unknown" {
			rows = append(rows, row{label: label, value: value})
		}
	}
	add("commit", version.Commit())
	add("branch", version.Branch)
	add("built", builtAgo(version.BuildTime()))
	add("built by", version.BuildBy)

	width := 0
	for _, r := range rows {
		width = max(width, len(r.label))
	}
	for _, r := range rows {
		if _, err := fmt.Fprintf(w, "  %-*s %s\n", width+1, r.label+":", r.value); err != nil {
			return err
		}
	}
	return nil
}

// builtAgo renders the RFC 3339 build time as a relative phrase ("2 hours
// ago"); a missing or unparseable time returns the input unchanged so the
// caller's unknown-filter handles it.
func builtAgo(buildTime string) string {
	t, err := time.Parse(time.RFC3339, buildTime)
	if err != nil {
		return buildTime
	}
	return human.FormatTimeAgo(t)
}
