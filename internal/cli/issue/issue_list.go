package issue

// issueListCommand lives in its own file so future per-verb evolutions
// (e.g. board scoping in 003) collide with a small, focused unit
// instead of the catch-all commands.go. Mirrors the
// `cmd/jira/issue_<verb>.go` convention established in 002
// (issue_attachment.go, issue_comment.go, issue_link.go,
// issue_watcher.go).

import (
	clib "github.com/gechr/clib/cli/cobra"
	"github.com/matcra587/jira-cli/internal/cli/boardscope"
	"github.com/matcra587/jira-cli/internal/cli/cmdutil"
	clijql "github.com/matcra587/jira-cli/internal/cli/jql"
	"github.com/spf13/cobra"
)

func issueListCommand() *cobra.Command {
	var opts issueListOptions
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List issues",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runIssueList(cmd, opts)
		},
	}
	cmd.Flags().BoolVar(&opts.detail, "detail", false, "Fetch full issue records")
	cmd.Flags().StringVar(&opts.jqlQuery, "jql", "", "Run a custom JQL query for the issue list")
	cmd.Flags().BoolVar(&opts.asJQL, "as-jql", false, "Print the built JQL query without calling Jira")
	cmd.Flags().BoolVar(&opts.count, "count", false, "Return only the approximate match count, without fetching issues")
	clijql.AddJQLBuilderFlags(cmd, &opts.builder)
	cmdutil.AddParallelismFlag(cmd, &opts.parallelism)
	cmdutil.AddIssueColumnFlags(cmd.Flags(), &opts.columns, &opts.tsv)
	boardscope.AddFlags(cmd)
	// --as-jql is an offline preview; --count calls Jira. They short-circuit
	// the runner differently, so they can't combine.
	cmd.MarkFlagsMutuallyExclusive("as-jql", "count")
	cmdutil.ExtendFlag(cmd.Flags(), "count", clib.FlagExtra{Group: "Output"})
	return cmd
}
