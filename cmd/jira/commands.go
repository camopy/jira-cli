package main

import (
	"os"

	"github.com/matcra587/jira-cli/internal/cli"
	"github.com/matcra587/jira-cli/internal/cli/alias"
	"github.com/matcra587/jira-cli/internal/cli/auth"
	"github.com/matcra587/jira-cli/internal/cli/boards"
	"github.com/matcra587/jira-cli/internal/cli/cache"
	"github.com/matcra587/jira-cli/internal/cli/config"
	"github.com/matcra587/jira-cli/internal/cli/epic"
	"github.com/matcra587/jira-cli/internal/cli/jql"
	"github.com/matcra587/jira-cli/internal/cli/me"
	"github.com/matcra587/jira-cli/internal/cli/search"
	"github.com/matcra587/jira-cli/internal/cli/version"
	"github.com/matcra587/jira-cli/internal/cli/worklog"
	"github.com/spf13/cobra"
)

func registerCommands(root *cobra.Command) {
	root.AddCommand(
		tuiCommand(),
		agentCommand(),
		cache.NewCommand(),
		me.NewCommand(),
		version.NewCommand(),
		auth.NewCommand(),
		issueCommand(),
		boards.NewCommand(),
		epic.NewCommand(),
		jql.NewCommand(),
		alias.NewCommand(),
		search.NewCommand(),
		worklog.NewCommand(),
		config.NewCommand(),
	)
}

func tuiCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "tui",
		Short:   "Launch the persistent dashboard",
		GroupID: "dashboard",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := cli.RequireTTY(os.Stdout)
			if err != nil {
				return err
			}
			_, err = tuiRun(cmd)
			return err
		},
	}
}
