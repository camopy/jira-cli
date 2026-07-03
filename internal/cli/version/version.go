// Package version implements the `jira version` cobra command, which prints
// build and version metadata as a structured envelope.
package version

import (
	"github.com/spf13/cobra"

	"github.com/matcra587/jira-cli/internal/cli/cmdutil"
	"github.com/matcra587/jira-cli/internal/version"
)

// NewCommand returns the `version` command: prints build/version metadata
// as a structured envelope.
func NewCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Example: `$ jira version

# Keep build metadata parseable for scripts
$ jira version --output=json`,
		GroupID: "agent",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmdutil.WriteEnvelope(cmd, "version", map[string]any{
				"version":    version.Version(),
				"commit":     version.Commit(),
				"branch":     version.Branch,
				"build_time": version.BuildTime(),
				"build_by":   version.BuildBy,
				"summary":    version.String(),
			})
		},
	}
}
