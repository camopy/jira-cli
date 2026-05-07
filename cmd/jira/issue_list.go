package main

// issueListCommand lives in its own file so future per-verb evolutions
// (e.g. board scoping in 003) collide with a small, focused unit
// instead of the catch-all commands.go. Mirrors the
// `cmd/jira/issue_<verb>.go` convention established in 002
// (issue_attachment.go, issue_comment.go, issue_link.go,
// issue_watcher.go).

import (
	clib "github.com/gechr/clib/cli/cobra"
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
	addJQLBuilderFlags(cmd, &opts.builder)
	addBoardScopeFlags(cmd)
	return cmd
}

// addBoardScopeFlags wires the `--board NAME` / `--board-id N` flag pair
// (with mutual exclusion) onto a list-style command. Shared by
// `issue list` and `jql build` so the surface stays in lockstep.
func addBoardScopeFlags(cmd *cobra.Command) {
	cmd.Flags().String("board", "", "Restrict to issues whose project belongs to the named board (case-insensitive exact match against the cache)")
	cmd.Flags().Int("board-id", 0, "Restrict to issues whose project belongs to the board with this id")
	cmd.MarkFlagsMutuallyExclusive("board", "board-id")
	clib.Extend(cmd.Flags().Lookup("board"), clib.FlagExtra{Group: "Filters", Placeholder: "NAME", Complete: "predictor=cacheboard"})
	clib.Extend(cmd.Flags().Lookup("board-id"), clib.FlagExtra{Group: "Filters", Placeholder: "N"})
}
